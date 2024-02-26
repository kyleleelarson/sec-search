package main

import (
  "os"
  "io"
  "fmt"
  "strconv"
  "bytes"
  "net/http"
  "html/template"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
  chartrender "github.com/go-echarts/go-echarts/v2/render"
)

const pageSz = 15 // rows in table to display
var client *ElasticClient
var templates = template.Must(template.ParseFiles("./html/table.html"))
var fields = [2]string {"Item1","Item1a"}
var years = [20]string {"2004","2005","2006","2007","2008","2009","2010","2011","2012","2013",
                        "2014","2015","2016","2017","2018","2019","2020","2021","2022","2023"}
type TableData struct {
  Years []string
  Fields []string
  Hits [](map[string]any)
}


type embedRender struct {
  c      interface{}
  before []func()
}

func NewEmbedRender(c interface{}, before ...func()) chartrender.Renderer {
  return &embedRender{c: c, before: before}
}

func (r *embedRender) Render(w io.Writer) error {
  dat, err := os.ReadFile("html/graph.html")
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

// helper functions to check request parameters and supply defaults
func paramStr(r *http.Request, name string, def string) string {
  var param string
  param = r.FormValue(name)
  if param == "" {
    return def
  }
  return param
}

func httpserver(w http.ResponseWriter, r *http.Request) {
  var (
    buf bytes.Buffer
    tableData TableData
  )
  barData := make(map[string]([]opts.BarData))

  searchTerm := r.FormValue("searchterm")
  stockIndex := paramStr(r, "stockindex", "S&P 500")
  section    := paramStr(r, "section", "Item1")
  year       := paramStr(r, "year", "2023")
  pageStr   := paramStr(r, "p", "0")

  page, err := strconv.Atoi(pageStr)
  if err != nil || page < 0 {
    http.Error(w, "invalid page parameter", http.StatusInternalServerError)
    return
  }

  counts, err := client.histogramSearch(searchTerm, stockIndex)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

  for _, field := range fields {
    var barValues []opts.BarData
    for _, year := range years {
      count := counts[field][year] // zero if not in map
      barValues = append(barValues, opts.BarData{Value: count})
    }
      barData[field] = barValues
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

  err = bar.Render(&buf)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

  tableData.Hits, err = client.highlightSearch(searchTerm, stockIndex, section, year, page, pageSz)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

  tableData.Years = years[:]
  tableData.Fields = fields[:]

  err = templates.ExecuteTemplate(&buf, "table.html", tableData)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
   
  fmt.Fprintf(w, "%s", buf.String())
}

func updateTable(w http.ResponseWriter, r *http.Request) {
  var (
    buf bytes.Buffer
    tableData TableData
    err error
  )

  searchTerm := r.FormValue("searchterm")
  stockIndex := paramStr(r, "stockindex", "S&P 500")
  section    := paramStr(r, "section", "Item1")
  year       := paramStr(r, "year", "2023")
  pageStr   := paramStr(r, "p", "0")

  page, err := strconv.Atoi(pageStr)
  if err != nil || page < 0 {
    http.Error(w, "invalid page parameter", http.StatusInternalServerError)
    return
  }

  tableData.Hits, err = client.highlightSearch(searchTerm, stockIndex, section, year, page, pageSz)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

  tableData.Years = years[:]
  tableData.Fields = fields[:]

  err = templates.ExecuteTemplate(&buf, "hits", tableData)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
   
  fmt.Fprintf(w, "%s", buf.String())
}

func main() {
  client = NewElasticClient()
	http.HandleFunc("/", httpserver)
	http.HandleFunc("/filter", updateTable)
	http.ListenAndServe(":8081", nil)
}
