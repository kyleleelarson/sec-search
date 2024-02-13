package main

import (
  "log"
  "os"
  "encoding/json"
  "strings"
  "github.com/elastic/go-elasticsearch/v8"
)

const indexName = "filings"

var es *elasticsearch.Client

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

  //query := `{"query":{"match":{"Item1":{"query":"artificial intelligence","operator":"AND"}}}}`
  //query := `{"query":{"match_phrase":{"Item1a":{"query":"mr. Musk", "analyzer":"standard"}}}}`
  query := `{"query":{"term":{"Industry.keyword":"Agricultural Inputs"}}}`
  //query := `{"query":{"range":{"Filed":{"gte":"2001-01","lte":"2002-01"}}}}`
  res, err := es.Search(
    es.Search.WithIndex(indexName),
    es.Search.WithBody(strings.NewReader(query)),
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
			log.Fatalf("[%s] %s: %s",
				res.Status(),
				e["error"].(map[string]interface{})["type"],
				e["error"].(map[string]interface{})["reason"],
			)
		}
	}

	var r  map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		log.Fatalf("Error parsing the response body: %s", err)
	}
	// Print the response status, number of results, and request duration.
	log.Printf(
		"[%s] %d hits; took: %dms",
		res.Status(),
		int(r["hits"].(map[string]interface{})["total"].(map[string]interface{})["value"].(float64)),
		int(r["took"].(float64)),
	)
	// Print the ID and document source for each hit.
  /*
	for _, hit := range r["hits"].(map[string]interface{})["hits"].([]interface{}) {
    if str, ok := hit.(map[string]interface{})["_source"].(map[string]interface{})["Item1"].(string); ok {
      log.Printf(" * ID=%s, %s", hit.(map[string]interface{})["_id"], str[:8])
	    log.Println(strings.Repeat("=", 37))
    }
	}
  */


}

