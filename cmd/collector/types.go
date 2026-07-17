package main

import (
	"net/http"

	"github.com/go-redis/redis/v8"
)

//ContextedHandler wrapper that prives the AppContext to the handlers
type ContextedHandler struct {
	*AppContext
	//ContextedHandlerFunc is the functions that the handlers must implement
	ContextedHandlerFunc func(*AppContext, http.ResponseWriter, *http.Request) (int, error)
}

//AppContext the struct that provides the context information
type AppContext struct {
	RedisClient *redis.Client
}
