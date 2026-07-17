package main

import (
	"net/http"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
)

func main() {
	router := mux.NewRouter()
	appContext := AppContext{
		RedisClient: redis.NewClient(&redis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
		}),
	}

	contextedHandler := ContextedHandler{
		AppContext:           &appContext,
		ContextedHandlerFunc: handleInfraAWS,
	}

	router.Path("/crawlers/infra/aws").Methods("POST").Handler(contextedHandler)

	http.ListenAndServe("localhost:8000", router)
}
