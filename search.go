package main

import (
  "log"
  "os"
  "io"
  "fmt"
  "encoding/json"
  "strings"
  "html/template"
  "github.com/elastic/go-elasticsearch/v8"
)

const indexName = "filings"

var histogramQuery = `{ 
  "size": 0,
  "query": { "bool": { 
    "must": [{ "match_phrase": { "%s": "%s" }}],
    "filter": [{ "bool": { "must": [{ "term": { "StockIndex.keyword": "%s" }}]}}]}},
  "aggs": { "year": { "date_histogram": { "field": "Filed", "calendar_interval": "1y"}}}
}`

type HistogramResult struct {
  Took float64 `json:"took"`
  Hits struct {
    Total struct {
      Num int `json:"value"`
    } `json:"total"`
  }
  Aggregations struct {
    Year struct {
      Buckets []struct {  
        Date  string  `json:"key_as_string"`
        Count float64 `json:"doc_count"`
      } `json:"buckets"`
    } `json:"year"`
  } `json:"aggregations"`
}

var highlightQuery = `{ 
  "_source": ["Ticker", "Name", "StockIndex", "Filed"],
  "query": { "bool": { 
    "must": [{ "match_phrase": { "%s": "%s" }}],
    "filter": [{ "bool": { "must": [{ "term": { "StockIndex.keyword": "%s" }}]}}]}},
  "highlight": { "fragment_size": 200, "fields": { "Item1": {}, "Item1a": {} } },
  "sort": [ { "Filed": { "order": "desc", "unmapped_type": "date" } } ],
  "size": 30
}`

type HighlightResult struct {
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
        Item1  []template.HTML // so <em> is not escaped
        Item1a []template.HTML
      } `json:"highlight"`
    } `json:"hits"`
  } `json:"hits"`
}

type ElasticClient struct {
  es *elasticsearch.Client
}

func NewElasticClient() *ElasticClient {
  cfg := elasticsearch.Config {
    Addresses: []string {
      "https://localhost:9200",
    },
    Username: "elastic",
    Password: os.Getenv("ES_PASS"),
    CertificateFingerprint: os.Getenv("ES_CERTFP"),
  }
  es, err := elasticsearch.NewClient(cfg)
  if err != nil {
    log.Fatalf("Error creating client: %s", err)
  }
  return &ElasticClient{es: es}
}

func (client *ElasticClient) histogramSearch(searchTerm, stockIndex string) (
  map[string](map[string]int), error) {

  var (
    err error
    histogramResult HistogramResult
  )
  counts := make(map[string](map[string]int))

  for _, field := range fields {
    m := make(map[string]int)
    res, err := client.es.Search(
      client.es.Search.WithIndex(indexName),
      client.es.Search.WithBody(strings.NewReader(
        fmt.Sprintf(histogramQuery, field, searchTerm, stockIndex))),
    )
    if err != nil {
      return counts, err
    }
    defer res.Body.Close()
    if res.IsError() || res.Status() != "200 OK" {
      err = fmt.Errorf("res.IsError or status not 200 OK")
        return counts, err
    }

    body, err := io.ReadAll(res.Body)
    if err != nil {
      log.Fatalf("Error reading the response body: %s", err)
      return counts, err
    }
    if err = json.Unmarshal(body, &histogramResult); err != nil {
      return counts, err
    }

    for _, b := range histogramResult.Aggregations.Year.Buckets {
      year := b.Date[:4]
      count := int(b.Count)
      m[year] = count
    }
  counts[field] = m
  }
  return counts, err
}

func (client *ElasticClient) highlightSearch(searchTerm, stockIndex, field string) (
  [](map[string]any), error) {

  var (
    hits [](map[string]any)
    err error
    highlightResult HighlightResult
  )

  res, err := client.es.Search(
    client.es.Search.WithIndex(indexName),
    client.es.Search.WithBody(strings.NewReader(
      fmt.Sprintf(highlightQuery, field, searchTerm, stockIndex))),
  )
  if err != nil {
    return hits, err
  }
  defer res.Body.Close()
  if res.IsError() || res.Status() != "200 OK" {
    err = fmt.Errorf("res.IsError or status not 200 OK")
      return hits, err
  }

  body, err := io.ReadAll(res.Body)
  if err != nil {
    log.Fatalf("Error reading the response body: %s", err)
    return hits, err
  }
  if err = json.Unmarshal(body, &highlightResult); err != nil {
    return hits, err
  }

  for _, hit := range highlightResult.Hits.Values {
    m := make(map[string]any)
    m["Filed"] = hit.Source.Filed
    m["Ticker"] = hit.Source.Ticker
    m["Name"] = hit.Source.Name
    if field == "Item1" {
      m["Excerpt"] = hit.Highlights.Item1[0]
    } else {
      m["Excerpt"] = hit.Highlights.Item1a[0]
    }
    hits = append(hits, m)
  }
  return hits, err
}
