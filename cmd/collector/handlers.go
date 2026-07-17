package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/coordimap/agent/internal/integrations/clouds"
)

func handleInfraAWS(c *AppContext, w http.ResponseWriter, r *http.Request) (int, error) {
	var crawledAWSData clouds.CloudCrawlData
	var allCrawledElements []*clouds.Element

	err := json.NewDecoder(r.Body).Decode(&crawledAWSData)
	if err != nil {
		fmt.Println("Error decoding the request: ", err.Error())
		return 500, err
	}

	errUnmarshal := json.Unmarshal(crawledAWSData.Data.Data, &allCrawledElements)
	if errUnmarshal != nil {
		fmt.Println(errUnmarshal.Error())
	}

	for _, elem := range allCrawledElements {
		// TODO: implement a function where for elem.Type I get the elem.Data and convert it to the corresponding JSON

		go func(elem *clouds.Element) {
			if c.RedisClient.Exists(context.Background(), elem.Hash).Err() != nil {
				c.RedisClient.Set(context.Background(), elem.Hash, "", 10*time.Hour)
			}

			c.RedisClient.Publish(context.Background(), "infra_crawl_aws", elem)
		}(elem)
	}

	w.WriteHeader(201)

	return 201, nil
}

func (handler ContextedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	status, err := handler.ContextedHandlerFunc(handler.AppContext, w, r)
	if err != nil {
		log.Printf("HTTP %d: %q", status, err)
		switch status {
		}
	}
}
