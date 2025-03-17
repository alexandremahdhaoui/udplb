package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/render"
	"github.com/go-echarts/go-echarts/v2/types"

	_ "github.com/marcboeker/go-duckdb"
)

func main() {
	// -- parse args
	csvPath, err := parseArgs()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	// -- perform data analysis
	analysis, err := AnalyzeData(csvPath)
	if err != nil {
		errExit(err)
	}

	// -- plot charts
	plot := NewPlot(analysis)

	page := components.NewPage()
	page.AddCharts(plot)

	// -- run server
	handler := NewHandleFunc(page)
	http.HandleFunc("/", handler)

	fmt.Println("open: http://127.0.0.1:8081")
	if err := http.ListenAndServe(":8081", nil); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		errExit(err)
	}
}

func errExit(err error) {
	slog.Error("encountered an unexpected error", "err", err.Error())
	os.Exit(1)
}

// ----------------------------------------------------------------------------------- //
// -- PARSE ARGS
// ----------------------------------------------------------------------------------- //

const usage = `USAGE:
	%s <CSV FILE PATH>`

func parseArgs() (string, error) {
	if len(os.Args) < 2 {
		return "", fmt.Errorf(usage, os.Args[0])
	}

	return os.Args[1], nil
}

// ----------------------------------------------------------------------------------- //
// -- DATA ANALYSIS
// ----------------------------------------------------------------------------------- //

func AnalyzeData(csvPath string) (any, error) {
	db, err := sql.Open("duckdb", "?access_mode=READ_WRITE")
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}

	createTable := `CREATE TABLE bench (
		algorithm VARCHAR,
		prime UBIGINT,
		nBefore UBIGINT,
		nAfter UBIGINT,
		execTime UBIGINT,
		unchangedEntries FLOAT,
		allocatedBytes UBIGINT,
		allocations UBIGINT,
	);`
	_, err = db.Exec(createTable)
	if err != nil {
		return nil, err
	}

	readCsv := fmt.Sprintf(`COPY bench FROM '%s'`, csvPath)
	_, err = db.Exec(readCsv)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(`SELECT * FROM bench`)
	if err != nil {
		return nil, err
	}

	// TODO: create jupyter notebooks, it will be easier.

	// i := 0
	for {
		if rows.Next() {
			var s string
			rows.Scan(s)
			fmt.Printf("%s\n", s)
			continue
		}

		break
	}

	return nil, nil
}

// ----------------------------------------------------------------------------------- //
// -- PLOT
// ----------------------------------------------------------------------------------- //

func NewPlot(_ any) components.Charter {
	// create a new ch instance
	ch := charts.NewBar()
	// set some global options like Title/Legend/ToolTip or anything else
	ch.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{Theme: types.ThemeWesteros}),
		charts.WithTitleOpts(opts.Title{
			Title:    "UDPLB Lookup Table Algorithm",
			Subtitle: "Analysis of UPDLB lookup tables",
		}))

	algo := []string{
		"NaiveSimple",
		"NaiveFib",
		"RobustSimple",
		"RobustFib",
	}

	columns := []string{
		"algorithm",
		"prime",
		"nBefore",
		"nAfter",
		"execTime",
		"unchangedEntries",
		"allocatedBytes",
		"allocations",
	}

	// Put data into instance
	ch.SetXAxis(columns).
		AddSeries(algo[0], generateBarItems()).
		AddSeries(algo[1], generateBarItems()).
		AddSeries(algo[2], generateBarItems()).
		AddSeries(algo[3], generateBarItems()).
		SetSeriesOptions(charts.WithLineChartOpts(opts.LineChart{Smooth: opts.Bool(true)}))

	// what to show on charts.
	// Axis X:
	// - Algorithms.
	// - Scenarios: nBefore/nAfter
	// - lookup table size: prime.
	// Axis Y:
	// - unchangedEntries.
	// - allocated bytes.
	// - allocations.

	return ch
}

// generate random data for line chart
func generateBarItems() []opts.BarData {
	items := make([]opts.BarData, 0)
	for i := 0; i < 9; i++ {
		items = append(items, opts.BarData{Value: rand.Intn(300)})
	}
	return items
}

// ----------------------------------------------------------------------------------- //
// -- HTTP HANDLER
// ----------------------------------------------------------------------------------- //

func NewHandleFunc(renderer render.Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := renderer.Render(w); err != nil {
			errExit(err)
		}
	}
}
