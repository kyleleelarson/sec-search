package main

import (
  "fmt"
  "math"
  "bytes"
  "strconv"
  "net/http"
  "html/template"
)

const pageSz = 15 // rows in table to display
var es *ElasticClient
var templates = template.Must(template.ParseFiles("./html/table.html"))
var sections = [2]string {"Item1","Item1a"}
var years = [20]string {"2004","2005","2006","2007","2008","2009","2010","2011","2012","2013",
                        "2014","2015","2016","2017","2018","2019","2020","2021","2022","2023"}

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
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

  err = templates.ExecuteTemplate(&buf, "hits", tableData)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
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
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

  err = renderGraph(counts, p, &buf)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

  tableData, err = prepareTable(p)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

  err = templates.ExecuteTemplate(&buf, "table.html", tableData)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
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
    p.stockIndex = paramStr(r, "stockindex", "S&P 500")
    p.section    = paramStr(r, "section", "Item1")
    p.year       = paramStr(r, "year", "2023")
    pageStr     := paramStr(r, "p", "1")

    p.page, err = strconv.Atoi(pageStr)
    if err != nil || p.page < 1 {
      http.Error(w, "invalid page parameter", http.StatusInternalServerError)
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
