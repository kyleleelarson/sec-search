package main

import (
  "log"
  "os"
  "io"
  "fmt"
  "encoding/json"
  "strings"
  "github.com/elastic/go-elasticsearch/v8"
)

const indexName = "filings"

var es *elasticsearch.Client


var histogramQuery = `{ 
        "size": 0,
        "query": { 
              "bool": { 
                "must": [{ "match_phrase": { "Item1": "%s" }}],
                "filter": [{ "bool": { "must": [{ "term": { "StockIndex.keyword": "%s" }}]}}]}},
        "aggs": { "year": { "date_histogram": { "field": "Filed", "calendar_interval": "1y"}}}
    }`

var highlightQuery = `{ 
              "_source": ["Ticker", "Name", "StockIndex", "Filed"],
              "query": { "match_phrase": { "Item1": "%s" } },
              "highlight": { "fragment_size": 200, "fields": { "Item1": {} } },
              "sort": [ { "Filed": { "order": "desc", "unmapped_type": "date" } } ],
              "size": 1000
            }`
// including highlight makes query ~3.5 times slower

type ResultBody struct {
  Took float64 `json:"took"`
  Hits struct {
    Total struct {
      Num int `json:"value"`
    } `json:"total"`
    Values []struct {  
      Id     string  `json:"_id"`
      Score  float64 `json:"_score"`
      Source struct {
        Ticker     string
        Name       string
        StockIndex string
        Filed      string
      } `json:"_source"`
      Highlights struct {
        Item1  []string
        Item1a []string
      } `json:"highlight"`
    } `json:"hits"`
  } `json:"hits"`
}

func client_init() {
  var err error

  cfg := elasticsearch.Config {
    Addresses: []string {
      "https://localhost:9200",
    },
    Username: "elastic",
    Password: os.Getenv("ES_PASS"),
    CertificateFingerprint: os.Getenv("ES_CERTFP"),
  }

  es, err = elasticsearch.NewClient(cfg)
  if err != nil {
    log.Fatalf("Error creating client: %s", err)
  }
}



func main() {
  client_init()

  res, err := es.Search(
    es.Search.WithIndex(indexName),
    es.Search.WithBody(strings.NewReader(fmt.Sprintf(highlightQuery, "blockchain"))),
  )
  if err != nil {
    log.Fatalf("Error getting response: %s", err)
  }
  defer res.Body.Close()
	if res.IsError() {
		var e map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
			log.Fatalf("Error parsing the response (with error) body: %s", err)
		} else {
			// Print the response status and error information.
			log.Fatalf("[%s] %s: %s",
				res.Status(),
				e["error"].(map[string]interface{})["type"],
				e["error"].(map[string]interface{})["reason"],
			)
		}
	}

  if res.Status() == "200 OK" {
    body, err := io.ReadAll(res.Body)
    if err != nil {
      log.Fatalf("Error reading the response body: %s", err)
    }
    var resultBody ResultBody
    if err = json.Unmarshal(body, &resultBody); err != nil {
      log.Fatalf("Error parsing the response body: %s", err)
    }
    log.Printf("took: %v ms\n", resultBody.Took)
    log.Printf("hits: %d\n", resultBody.Hits.Total.Num)
    for i, hit := range resultBody.Hits.Values {
      log.Printf("  id: %s, filed: %s, ticker: %s, index: %s\n", hit.Id, hit.Source.Filed,
                  hit.Source.Ticker, hit.Source.StockIndex)
      if i > 10 {
        break
      }
    }
  }

  res, err = es.Search(
    es.Search.WithIndex(indexName),
    es.Search.WithBody(strings.NewReader(fmt.Sprintf(histogramQuery, "blockchain", "RUSSELL 2000"))),
    es.Search.WithPretty(),
  )
  if err != nil {
    log.Fatalf("Error getting response: %s", err)
  }
  defer res.Body.Close()
	if res.IsError() {
		var e map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
			log.Fatalf("Error parsing the response (with error) body: %s", err)
		} else {
			// Print the response status and error information.
			log.Fatalf("res.IsError [%s] %s: %s",
				res.Status(),
				e["error"].(map[string]interface{})["type"],
				e["error"].(map[string]interface{})["reason"],
			)
		}
	}
}
