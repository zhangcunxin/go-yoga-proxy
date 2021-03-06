package main

import (
	"flag"
	"fmt"
	"github.com/bingoohuang/go-utils"
	"github.com/go-redis/redis"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	contextPath string
	port        string
	redisServer RedisServer
)

func init() {
	contextPathArg := flag.String("contextPath", "", "context path")
	redisAddrArg := flag.String("redisAddr", "127.0.0.1:6379", "redis server addr")
	portArg := flag.Int("port", 8082, "Port to serve.")

	flag.Parse()

	contextPath = *contextPathArg
	if contextPath != "" && strings.Index(contextPath, "/") < 0 {
		contextPath = "/" + contextPath
	}

	port = strconv.Itoa(*portArg)
	redisServer = parseServerItem(*redisAddrArg)
}

func main() {
	r := mux.NewRouter()

	handleFunc(r, "/clearCache", clearCache, false)
	handleFunc(r, "/getCache", getCache, false)
	handleFunc(r, "/setCache", setCache, false)
	handleFunc(r, "/zaddCache", zaddCache, false)
	http.Handle("/", r)

	fmt.Println("start to listen at ", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func handleFunc(r *mux.Router, path string, f func(http.ResponseWriter, *http.Request), requiredGzip bool) {
	wrap := go_utils.DumpRequest(f)

	if requiredGzip {
		wrap = go_utils.GzipHandlerFunc(wrap)
	}

	r.HandleFunc(contextPath+path, wrap)
}

func zaddCache(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.FormValue("key"))
	value := strings.TrimSpace(r.FormValue("value"))
	score := strings.TrimSpace(r.FormValue("score"))
	db, _ := strconv.Atoi(go_utils.EmptyThen(r.FormValue("db"), "0"))

	i, err := zaddRedisContent(key, value, score, db)
	if err != nil {
		http.Error(w, err.Error(), 405)
		return
	}

	w.Write([]byte(strconv.FormatInt(i, 10)))
}

func setCache(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.FormValue("key"))
	value := strings.TrimSpace(r.FormValue("value"))
	ttl := strings.TrimSpace(r.FormValue("ttl"))
	db, _ := strconv.Atoi(go_utils.EmptyThen(r.FormValue("db"), "0"))

	val, err := setRedisContent(key, value, ttl, db)
	if err != nil {
		http.Error(w, err.Error(), 405)
		return
	}
	w.Write([]byte(val))
}

func getCache(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.FormValue("key"))
	db, _ := strconv.Atoi(go_utils.EmptyThen(r.FormValue("db"), "0"))

	val, err := getRedisContent(key, db)
	if err == redis.Nil {
		w.Write([]byte(""))
		return
	}

	if err != nil {
		http.Error(w, err.Error(), 405)
		return
	}

	w.Write([]byte(val))
}

func clearCache(w http.ResponseWriter, r *http.Request) {
	keys := strings.TrimSpace(r.FormValue("keys"))
	db, _ := strconv.Atoi(go_utils.EmptyThen(r.FormValue("db"), "0"))

	log.Println("clear cache for keys:", keys)

	result, err := deleteMultiKeys(strings.Split(keys, ","), db)
	if err != nil {
		http.Error(w, err.Error(), 405)
		return
	}

	w.Write([]byte(strconv.FormatInt(result, 10)))
}

type RedisServer struct {
	Addr      string
	Password  string
	DefaultDb int
}

func splitTrim(str, sep string) []string {
	subs := strings.Split(str, sep)
	ret := make([]string, 0)
	for i, v := range subs {
		v := strings.TrimSpace(v)
		if len(subs[i]) > 0 {
			ret = append(ret, v)
		}
	}

	return ret
}

// password2/localhost:6388/0

func parseServerItem(serverConfig string) RedisServer {
	serverItems := splitTrim(serverConfig, "/")
	len := len(serverItems)
	if len == 1 {
		return RedisServer{
			Addr:      serverItems[0],
			Password:  "",
			DefaultDb: 0,
		}
	} else if len == 2 {
		dbIndex, _ := strconv.Atoi(serverItems[1])
		return RedisServer{
			Addr:      serverItems[0],
			Password:  "",
			DefaultDb: dbIndex,
		}
	} else if len == 3 {
		dbIndex, _ := strconv.Atoi(serverItems[2])
		return RedisServer{
			Addr:      serverItems[1],
			Password:  serverItems[0],
			DefaultDb: dbIndex,
		}
	} else {
		panic("invalid servers argument")
	}
}

func newRedisClientToDb(server RedisServer, db int) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     server.Addr,
		Password: server.Password, // no password set
		DB:       db,
	})
}

func getRedisContent(key string, db int) (string, error) {
	client := newRedisClientToDb(redisServer, db)
	defer client.Close()

	return client.Get(key).Result()
}

func zaddRedisContent(key, value, score string, db int) (int64, error) {
	client := newRedisClientToDb(redisServer, db)
	defer client.Close()

	f, e := strconv.ParseFloat(score, 64)
	if e != nil {
		return 0, e
	}

	return client.ZAdd(key, redis.Z{
		Score:  f,
		Member: value,
	}).Result()
}

func setRedisContent(key, value, ttl string, db int) (string, error) {
	client := newRedisClientToDb(redisServer, db)
	defer client.Close()

	duration, err := time.ParseDuration(ttl)
	if err != nil {
		return "", err
	}

	return client.Set(key, value, duration).Result()
}

func deleteMultiKeys(keys []string, db int) (int64, error) {
	client := newRedisClientToDb(redisServer, db)
	defer client.Close()

	return client.Del(keys...).Result()
}
