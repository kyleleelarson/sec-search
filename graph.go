package main

import (
  "os"
  "io"
  "fmt"
  "bytes"
  "html/template"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/charts"
  chartrender "github.com/go-echarts/go-echarts/v2/render"
)

/* code to embed echarts graphs in templates adapted from 
 * https://blog.cubieserver.de/2020/how-to-render-standalone-html-snippets-with-go-echarts/
 * under https://creativecommons.org/licenses/by/4.0/ */
type embedRender struct {
  c      interface{}
  before []func()
}

func NewEmbedRender(c interface{}, before ...func()) chartrender.Renderer {
  return &embedRender{c: c, before: before}
}

func (r *embedRender) Render(w io.Writer) error {
  data, err := os.ReadFile("html/graph.html")
  baseTpl := string(data)
  const tplName = "chart"
  for _, fn := range r.before {
    fn()
  }

  tpl := template.
    Must(template.New(tplName). // must wraps a func returning a template, panics if err
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

func renderGraph(counts map[string](map[string]int), p *Parameters, buf *bytes.Buffer) error {
  barData := make(map[string]([]opts.BarData))

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

  err := bar.Render(buf)
  return err
}
