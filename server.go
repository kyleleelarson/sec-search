package main

import (
  "os"
  "io"
  "fmt"
  "math"
  "bytes"
  "strconv"
  "net/http"
  "html/template"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/charts"
  chartrender "github.com/go-echarts/go-echarts/v2/render"
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
  barData := make(map[string]([]opts.BarData))

  counts, err := es.histogramSearch(p.searchTerm, p.stockIndex)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

  for _, section := range sections {
    var barValues []opts.BarData
    for _, year := range years {
      count := counts[section][year] // zero if not in map
      barValues = append(barValues, opts.BarData{Value: count})
    }
      barData[section] = barValues
  }

	// create a new bar instance
	bar := charts.NewBar()
  bar.Renderer = NewEmbedRender(bar, bar.Validate)
	// set some global options like Title/Legend/ToolTip or anything else
	bar.SetGlobalOptions(charts.WithTitleOpts(opts.Title{
		Title:    p.searchTerm,
		Subtitle: p.stockIndex,
	}))

	// Put data into instance
	bar.SetXAxis(years).
		AddSeries(sections[0], barData[sections[0]]).
		AddSeries(sections[1], barData[sections[1]]).
    SetSeriesOptions(charts.WithBarChartOpts(opts.BarChart{
		  Stack: "stackA",
		}))

  err = bar.Render(&buf)
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
