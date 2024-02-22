package main

import (
  "log"
  "os"
  "io"
  "fmt"
  "bytes"
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
var templates = template.Must(template.ParseFiles("table.html"))
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
var highlightQuery = `{ 
              "_source": ["Ticker", "Name", "StockIndex", "Filed"],
              "query": { 
                    "bool": { 
                      "must": [{ "match_phrase": { "%s": "%s" }}],
                      "filter": [{ "bool": { "must": [{ "term": { "StockIndex.keyword": "%s" }}]}}]}},
              "highlight": { "fragment_size": 200, "fields": { "Item1": {} } },
              "sort": [ { "Filed": { "order": "desc", "unmapped_type": "date" } } ],
              "size": 30
            }`

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
        Item1  []template.HTML
        Item1a []template.HTML
      } `json:"highlight"`
    } `json:"hits"`
  } `json:"hits"`
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
  stockIndex := "S&P 500"

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

  var buf bytes.Buffer

  err := bar.Render(&buf)
  if err != nil {
    log.Fatalf("Error rendering into buffer: %s", err)
  }

  res, err := es.Search(
    es.Search.WithIndex(indexName),
    es.Search.WithBody(strings.NewReader(fmt.Sprintf(highlightQuery, "Item1", searchTerm, stockIndex))),
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
      if i > 1 {
        break
      }
    }
    err = templates.ExecuteTemplate(&buf, "table.html", resultBody)
    if err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
    }
  }
  /*
  page := template.HTML(buf.String())
  finalTpl := template.Must(template.New("final").Parse(`{{.}}`))
  finalTpl.Execute(w, page)
  */
  fmt.Fprintf(w, "%s", buf.String())
}

func main() {
  client_init()
	http.HandleFunc("/", httpserver)
	//http.HandleFunc("/search", httpserver)
	http.ListenAndServe(":8081", nil)
}
