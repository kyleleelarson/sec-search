package main

import (
  "log"
  "os"
  "io"
  "fmt"
  "net/http"
  "html/template"
  "encoding/json"
  "strings"
  "github.com/elastic/go-elasticsearch/v8"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
  chartrender "github.com/go-echarts/go-echarts/v2/render"
)

const indexName = "filings"
var fields = [2]string {"Item1","Item1a"}
var years = [20]string {"2004","2005","2006","2007","2008","2009","2010","2011","2012","2013",
                        "2014","2015","2016","2017","2018","2019","2020","2021","2022","2023"}

var es *elasticsearch.Client

var histogramQuery = `{ 
        "size": 0,
        "query": { 
              "bool": { 
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

type embedRender struct {
  c      interface{}
  before []func()
}

func NewEmbedRender(c interface{}, before ...func()) chartrender.Renderer {
  return &embedRender{c: c, before: before}
}

func (r *embedRender) Render(w io.Writer) error {
  dat, err := os.ReadFile("index.html")
  baseTpl := string(dat)
  const tplName = "chart"
  for _, fn := range r.before {
    fn()
  }

  tpl := template.
    Must(template.New(tplName). // must is a wrapper for a func returning a template, panics if err
      Funcs(template.FuncMap{
        "safeJS": func(s interface{}) template.JS {
          return template.JS(fmt.Sprint(s)) // concatenates and casts to type JS
        },
      }).
      Parse(baseTpl),
    )
  err = tpl.ExecuteTemplate(w, tplName, r.c)
  return err
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



func httpserver(w http.ResponseWriter, r *http.Request) {
  searchTerm := r.FormValue("searchterm")
  barData    := make(map[string][]opts.BarData)
  stockIndex := "RUSSELL 2000"

  for _, field := range fields {
    res, err := es.Search(
      es.Search.WithIndex(indexName),
      es.Search.WithBody(strings.NewReader(fmt.Sprintf(histogramQuery, field, searchTerm, stockIndex))),
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

    if res.Status() == "200 OK" {
      body, err := io.ReadAll(res.Body)
      if err != nil {
        log.Fatalf("Error reading the response body: %s", err)
      }
      var result HistogramResult
      if err = json.Unmarshal(body, &result); err != nil {
        log.Fatalf("Error parsing the response body: %s", err)
      }
      log.Printf("For field %s, ", field)
      log.Printf("took: %v ms, ", result.Took)
      log.Printf("hits: %d\n", result.Hits.Total.Num)

      counts := make(map[string]int)
      for _, b := range result.Aggregations.Year.Buckets {
        year := b.Date[:4]
        count := int(b.Count)
        //log.Printf("  date: %s, count: %v\n", year, count)
        counts[year] = count
      }

      for _, year := range years {
        count := counts[year] // zero if not in map
        barData[field] = append(barData[field], opts.BarData{Value: count})
      }
    }
  }

	// create a new bar instance
	bar := charts.NewBar()
  bar.Renderer = NewEmbedRender(bar, bar.Validate)
	// set some global options like Title/Legend/ToolTip or anything else
	bar.SetGlobalOptions(charts.WithTitleOpts(opts.Title{
		Title:    searchTerm,
		Subtitle: stockIndex,
	}))

	// Put data into instance
	bar.SetXAxis(years).
		AddSeries(fields[0], barData[fields[0]]).
		AddSeries(fields[1], barData[fields[1]]).
    SetSeriesOptions(charts.WithBarChartOpts(opts.BarChart{
		  Stack: "stackA",
		}))

	bar.Render(w)
}

func main() {
  client_init()
	http.HandleFunc("/", httpserver)
	//http.HandleFunc("/search", httpserver)
	http.ListenAndServe(":8081", nil)
}
