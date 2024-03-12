package main

import (
  "fmt"
  "log"
  "math"
  "bytes"
  "strconv"
  "net/http"
  "html/template"
)

const pageSz = 15 // rows in table to display
var es *ElasticClient
var templates = template.Must(template.ParseFiles("./html/table.html"))
var sections = [2]string {"1. Business","1A. Risk Factors"}
var years = [20]string {"2005","2006","2007","2008","2009","2010","2011","2012","2013","2014",
                        "2015","2016","2017","2018","2019","2020","2021","2022","2023","2024"}
const yearUpperBound = 2024
const yearLowerBound = 2005
const defaultYear       = "2024"
const defaultStockIndex = "S&P 500"
const defaultSection    = "1. Business"
const defaultPage       = "1"

// struct of query string parameters to pass around                        
type Parameters struct {
  searchTerm string   
  stockIndex string   
  section    string   
  year       string   
  page       int   
}

type TableData struct {
  Page  int
  Pages int
  Year    string
  Section string
  Years    []string
  Sections []string
  Hits [](map[string]any)
}

func prepareTable(p *Parameters) (*TableData, error) {
  var (
    total int
    tableData TableData
    err error
  )

  total, tableData.Hits, err = es.highlightSearch(p.searchTerm, p.stockIndex, 
                                                            p.section, p.year, p.page, pageSz)
  if err != nil {
    return &tableData, err
  }

  tableData.Page = p.page
  tableData.Pages = int(math.Ceil(float64(total) / float64(pageSz)))
  tableData.Year = p.year
  tableData.Section = p.section
  for _, y := range years {
    if y != p.year {
      tableData.Years = append(tableData.Years, y)
    }
  }
  for _, s := range sections {
    if s != p.section {
      tableData.Sections = append(tableData.Sections, s)
    }
  }

  return &tableData, err
}

func updateTable(w http.ResponseWriter, r *http.Request, p *Parameters) {
  var (
    buf bytes.Buffer
    tableData *TableData
    err error
  )

  tableData, err = prepareTable(p)
  if err != nil {
    http.Error(w, "prepare table error", http.StatusInternalServerError)
    log.Printf("in update table with search term '%s', prepare table error: %s\n", 
      p.searchTerm, err.Error())
    return
  }

  err = templates.ExecuteTemplate(&buf, "hits", tableData)
  if err != nil {
    http.Error(w, "template error", http.StatusInternalServerError)
    log.Printf("in update table with search term '%s', execute template error: %s\n",
      p.searchTerm, err.Error())
    return
  }
   
  fmt.Fprintf(w, "%s", buf.String())
}

func httpserver(w http.ResponseWriter, r *http.Request, p *Parameters) {
  var (
    buf bytes.Buffer
    tableData *TableData
    err error
  )

  counts, err := es.histogramSearch(p.searchTerm, p.stockIndex)
  if err != nil {
    http.Error(w, "histogram search error", http.StatusInternalServerError)
    log.Printf("in httpserver with search term '%s', histogram search error: %s\n",
      p.searchTerm, err.Error())
    return
  }

  err = renderGraph(counts, p, &buf)
  if err != nil {
    http.Error(w, "graph render error", http.StatusInternalServerError)
    log.Printf("in httpserver with search term '%s', render graph error: %s\n",
      p.searchTerm, err.Error())
    return
  }

  tableData, err = prepareTable(p)
  if err != nil {
    http.Error(w, "prepare table error", http.StatusInternalServerError)
    log.Printf("in httpserver with search term '%s', prepare table error: %s\n",
      p.searchTerm, err.Error())
    return
  }

  err = templates.ExecuteTemplate(&buf, "table.html", tableData)
  if err != nil {
    http.Error(w, "template error", http.StatusInternalServerError)
    log.Printf("in httpserver with search term '%s', execute template error: %s\n",
      p.searchTerm, err.Error())
    return
  }
   
  fmt.Fprintf(w, "%s", buf.String())
}

// helper function to check request parameters and supply defaults
func paramStr(r *http.Request, name string, def string) string {
  var param string
  param = r.FormValue(name)
  if param == "" {
    return def
  }
  return param
}

func processParameters(fn func (http.ResponseWriter, *http.Request, *Parameters)) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    var (
      p Parameters
      err error
    )

    p.searchTerm = r.FormValue("searchterm")
    p.stockIndex = paramStr(r, "stockindex", defaultStockIndex)
    p.section    = paramStr(r, "section",    defaultSection)
    p.year       = paramStr(r, "year",       defaultYear)
    pageStr     := paramStr(r, "p",          defaultPage)

    p.page, err = strconv.Atoi(pageStr)
    if err != nil || p.page < 1 {
      http.Error(w, "invalid page parameter", http.StatusBadRequest)
      return
    }

    fn(w, r, &p);
  }
}

func main() {
  es = NewElasticClient()
	http.HandleFunc("/", processParameters(httpserver))
	http.HandleFunc("/filter", processParameters(updateTable))
	http.ListenAndServe(":8081", nil)
}
