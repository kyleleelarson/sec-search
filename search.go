package main

import (
  "log"
  "os"
  "io"
  "fmt"
  "encoding/json"
  "strings"
  "strconv"
  "html/template"
  "github.com/elastic/go-elasticsearch/v8"
)

const indexName = "filings_2024_03_11"

var histogramQuery = `{ 
  "size": 0,
  "query": { "bool": { 
    "must": [{ "match_phrase": { "%s": "%s" }}],
    "filter": [{ "term": { "StockIndex.keyword": "%s" }}]}},
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
  "_source": ["Ticker", "Name", "StockIndex", "Filed", "Url"],
  "query": { "bool": { 
    "must": [{ "match_phrase": { "%s": "%s" }}],
    "filter": [{ "term": { "StockIndex.keyword": "%s" }},
               { "range": { "Filed": { "gt": "%s", "lt": "%s"}}}]}},
  "highlight": { "fragment_size": 200, "fields": { "1. Business": {}, "1A. Risk Factors": {} } },
  "sort": [ { "Filed": { "order": "desc", "unmapped_type": "date" } } ],
  "from": %d,
  "size": %d
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
        Url        string
      } `json:"_source"`
      Highlights struct {
        Item1  []template.HTML `json:"1. Business"`     // so <em> is not escaped
        Item1a []template.HTML `json:"1A. Risk Factors"`
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
      os.Getenv("ES_ADDR"),
    },
    Username: "elastic",
    Password: os.Getenv("ES_PASS"),
    CertificateFingerprint: os.Getenv("ES_CERTFP"),
  }
  es, err := elasticsearch.NewClient(cfg)
  if err != nil {
    log.Fatalf("Error creating client: %s", err)
  }
  // check info to see if es is up and running
  res, err := es.Info()
  if err != nil || res.IsError() {
    log.Fatalf("Error checking es info: %s", err)
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

  for _, section := range sections {
    m := make(map[string]int)
    res, err := client.es.Search(
      client.es.Search.WithIndex(indexName),
      client.es.Search.WithBody(strings.NewReader(
        fmt.Sprintf(histogramQuery, section, searchTerm, stockIndex))),
    )
    if err != nil {
      return counts, err
    }
    defer res.Body.Close()
    if res.IsError() || res.Status() != "200 OK" {
      err = fmt.Errorf("status not 200 OK or res.IsError: %s", res.String())
      return counts, err
    }

    body, err := io.ReadAll(res.Body)
    if err != nil {
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
  counts[section] = m
  }
  return counts, err
}

func processYear(year string) (string, string) {
  i, err := strconv.Atoi(year)
  if err != nil || i < yearLowerBound || i > yearUpperBound {
    return strconv.Itoa(yearLowerBound) + "-12-31", strconv.Itoa(yearUpperBound) + "-01-01"
  }
  return strconv.Itoa(i-1) + "-12-31", strconv.Itoa(i+1) + "-01-01"
}

func (client *ElasticClient) highlightSearch(searchTerm, stockIndex, section, year string, 
  page, size int) (int, [](map[string]any), error) {

  var (
    total = 0
    hits [](map[string]any)
    err error
    highlightResult HighlightResult
  )

  yearLower, yearUpper := processYear(year)

  res, err := client.es.Search(
    client.es.Search.WithIndex(indexName),
    client.es.Search.WithBody(strings.NewReader(
      fmt.Sprintf(highlightQuery, section, searchTerm, stockIndex, yearLower, yearUpper, 
      (page-1) * size, size))),
  )
  if err != nil {
    return total, hits, err
  }
  defer res.Body.Close()
  if res.IsError() || res.Status() != "200 OK" {
    err = fmt.Errorf("status not 200 OK or res.IsError: %s", res.String())
    return total, hits, err
  }

  body, err := io.ReadAll(res.Body)
  if err != nil {
    return total, hits, err
  }
  if err = json.Unmarshal(body, &highlightResult); err != nil {
    return total, hits, err
  }

  total = highlightResult.Hits.Total.Num

  for _, hit := range highlightResult.Hits.Values {
    m := make(map[string]any)
    m["Filed"] = hit.Source.Filed
    m["Ticker"] = hit.Source.Ticker
    m["Name"] = hit.Source.Name
    m["Url"] = hit.Source.Url
    if section == "1. Business" {
      m["Excerpt"] = hit.Highlights.Item1[0]
    } else {
      m["Excerpt"] = hit.Highlights.Item1a[0]
    }
    hits = append(hits, m)
  }
  return total, hits, err
}
