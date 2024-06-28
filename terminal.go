package main

import (
	"context"
	"fmt"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/woodliu/termdash"
	"github.com/woodliu/termdash/cell"
	"github.com/woodliu/termdash/container"
	"github.com/woodliu/termdash/container/grid"
	"github.com/woodliu/termdash/keyboard"
	"github.com/woodliu/termdash/linestyle"
	"github.com/woodliu/termdash/terminal/tcell"
	"github.com/woodliu/termdash/terminal/terminalapi"
	"github.com/woodliu/termdash/widgets/linechart"
	"github.com/woodliu/termdash/widgets/text"
	"github.com/woodliu/termdash/widgets/textinput"
	"go.uber.org/atomic"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	fullPerc            = 99
	paddingVerticalPerc = 50
	legendCharacter     = `⠤⠤`
	axesColor           = 8
	yAxisLabelsColor    = 15
	xAxisLabelsColor    = 248
)

type graph struct {
	q                 *querier
	isSyncing         atomic.Bool
	widgetExprInput   *textinput.TextInput
	widgetExprLegend  *text.Text
	widgetGraph       *linechart.LineChart
	widgetGraphLegend *text.Text
	exprUpdateCh      chan string
	expr              atomic.String
}

func newGraph() (*graph, error) {
	exprUpdateCh := make(chan string)
	exprLegend, err := text.New(text.WrapAtRunes())
	if err != nil {
		return nil, err
	}

	exprInput, err := newTextInput(exprUpdateCh, exprLegend)
	if err != nil {
		return nil, err
	}

	// Create the Graph widget.
	lc, err := newLineChart()
	if err != nil {
		return nil, err
	}

	lineTxt, err := text.New(text.WrapAtRunes())
	if err != nil {
		return nil, err
	}

	return &graph{
		q:                 newQuerier(storageDir, localStorage),
		widgetExprInput:   exprInput,
		widgetExprLegend:  exprLegend,
		widgetGraph:       lc,
		widgetGraphLegend: lineTxt,
		exprUpdateCh:      exprUpdateCh,
	}, nil
}

func newLineChart() (*linechart.LineChart, error) {
	return linechart.New(
		linechart.AxesCellOpts(cell.FgColor(cell.ColorNumber(axesColor))),
		linechart.YLabelCellOpts(cell.FgColor(cell.ColorNumber(yAxisLabelsColor))),
		linechart.XLabelCellOpts(cell.FgColor(cell.ColorNumber(xAxisLabelsColor))),
		linechart.YAxisAdaptive(),
		linechart.YAxisFormattedValues(func(value float64) string {
			return shortFormatter(value, 2)
		}),
	)
}

// newTextInput creates a new TextInput field that changes the text on the
// SegmentDisplay.
func newTextInput(updateExpr chan<- string, txt *text.Text) (*textinput.TextInput, error) {
	input, err := textinput.New(
		textinput.Label("", cell.FgColor(cell.ColorNumber(1))),
		textinput.WidthPerc(100),
		textinput.FillColor(cell.ColorNumber(255)),
		textinput.PlaceHolder("Enter a PromQL query..."),
		textinput.PlaceHolderColor(8),
		textinput.OnSubmit(func(expr string) error {
			updateExpr <- expr
			txt.Write(fmt.Sprintf("%s %s  ", legendCharacter, expr), text.WriteReplace())

			return nil
		}),
		textinput.ClearOnSubmit(),
	)
	if err != nil {
		return nil, err
	}
	return input, err
}

const (
	termRedrawInterval        = time.Second * 5
	graphRefreshInterval      = time.Second * 10
	graphPointQuantityRetries = 5
)

func createDashboard(ctx context.Context, cancel func()) error {
	gr, err := newGraph()
	if err != nil {
		return err
	}

	go func() {
		builder := grid.New()
		builder.Add(elementFromGraphAndLegend(gr.widgetExprInput, gr.widgetExprLegend, gr.widgetGraph, gr.widgetGraphLegend))
		gridOpts, err := builder.Build()
		if err != nil {
			zapLogger.Panicln(err)
		}

		t, err := tcell.New(tcell.ColorMode(terminalapi.ColorMode256))
		if err != nil {
			zapLogger.Panicln(err)
		}
		defer t.Close()

		cont, err := container.New(t, gridOpts...)
		if err != nil {
			zapLogger.Panicln(err)
		}

		quitter := func(k *terminalapi.Keyboard) {
			if k.Key == keyboard.KeyEsc || k.Key == keyboard.KeyCtrlC {
				cancel()
			}
		}

		if err := termdash.Run(ctx, t, cont, termdash.KeyboardSubscriber(quitter), termdash.RedrawInterval(termRedrawInterval)); err != nil {
			zapLogger.Errorf("error running termdash terminal: %s", err)
		}
	}()

	go gr.run(ctx)

	return nil
}

func elementFromGraphAndLegend(exprInput *textinput.TextInput, exprTxt *text.Text, graph *linechart.LineChart, legend *text.Text) grid.Element {
	exprTxtElement := grid.Widget(exprTxt)
	graphElement := grid.Widget(graph)

	var elements []grid.Element
	legendElement := grid.RowHeightPercWithOpts(
		fullPerc,
		[]container.Option{container.PaddingTopPercent(paddingVerticalPerc)},
		grid.Widget(legend))

	elements = []grid.Element{
		grid.RowHeightPerc(10,
			grid.Widget(exprInput,
				container.Border(linestyle.Light),
				container.BorderTitle("Press Esc to quit"),
			),
		),
		grid.RowHeightPerc(4, exprTxtElement),
		grid.RowHeightPerc(80, graphElement),
		grid.RowHeightPerc(4, legendElement),
	}

	opts := []container.Option{
		container.Border(linestyle.Light),
	}

	return grid.RowHeightPercWithOpts(fullPerc, opts, elements...)
}

func (g *graph) run(ctx context.Context) {
	tk := time.NewTicker(graphRefreshInterval)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
		case expr := <-g.exprUpdateCh:
			g.expr.Store(expr)
			g.widgetGraph.Reset()
		}

		err := g.sync(ctx)
		if err != nil {
			g.widgetExprLegend.Write(fmt.Sprintf("error:%s", err.Error()), text.WriteReplace())
		}
	}
}

func (g *graph) sync(ctx context.Context) error {
	if !g.isSyncing.CompareAndSwap(false, true) {
		return nil
	}
	defer g.isSyncing.Store(false)

	// Get the max capacity of render points (this will be the number of metrics retrieved
	// for the range) of the X axis.
	// If we don't have capacity then return as a dummy sync1 (no error).
	capacity := g.getWindowCapacity()
	if capacity <= 0 {
		return nil
	}

	// Gather metrics from multiple queries.
	end := time.Now()
	start := end.Add(-RetentionDuration)
	step := end.Sub(start) / time.Duration(capacity)
	series, err := g.queryRange(ctx, g.expr.Load(), start, step)
	if err != nil {
		return err
	}

	// Merge sort all series.
	metrics := g.sortSeries(series)

	// Transform metric to the ones the render part understands.
	xLabels, indexedTime := g.createIndexedSlices(start, end, step, capacity)
	transSeries := g.transformToRenderable(metrics, xLabels, indexedTime)

	// Reset legend on each sync1.
	g.widgetGraphLegend.Reset()

	for _, s := range transSeries {
		// We fail all the graph sync1 if one of the series fail.
		err := g.syncSeries(s)
		if err != nil {
			return err
		}
	}
	return nil
}

// syncSeries will sync1 the widgets with one of the series.
func (g *graph) syncSeries(series Series) error {
	color, err := colorHexToTermdash(series.Color)
	if err != nil {
		return err
	}

	err = g.syncGraph(series, color)
	if err != nil {
		return err
	}

	err = g.syncLegend(series, color)
	if err != nil {
		return err
	}

	return nil
}

func colorHexToTermdash(color string) (cell.Color, error) {
	c, err := colorful.Hex(color)
	if err != nil {
		return 0, fmt.Errorf("error getting color: %s", err)
	}

	cr, cg, cb := c.RGB255()
	return cell.ColorRGB24(int(cr), int(cg), int(cb)), nil
}

// syncGraph will set one series of metrics on the graph.
func (g *graph) syncGraph(series Series, color cell.Color) error {
	// Convert to float64 values.
	values := make([]float64, len(series.Values))
	for i, value := range series.Values {
		// Use NaN as no value for Termdash.
		v := math.NaN()
		if value != nil {
			v = float64(*value)
		}
		values[i] = v
	}

	// Sync widget.
	err := g.widgetGraph.Series(series.Label, values,
		linechart.SeriesCellOpts(cell.FgColor(color)),
		linechart.SeriesXLabels(xLabelsSliceToMap(series.XLabels)))
	if err != nil {
		return err
	}

	return nil
}

func xLabelsSliceToMap(labels []string) map[int]string {
	mlabel := map[int]string{}
	for i, label := range labels {
		mlabel[i] = label
	}
	return mlabel
}

// syncLegend will set the legend if required and with the correct format.
func (g *graph) syncLegend(series Series, color cell.Color) error {
	legend := fmt.Sprintf("%s %s  ", legendCharacter, series.Label)

	// Write the legend on the widget.
	err := g.widgetGraphLegend.Write(legend, text.WriteCellOpts(cell.FgColor(color)))
	if err != nil {
		return err
	}

	return nil
}

func (g *graph) queryRange(ctx context.Context, query string, start time.Time, step time.Duration) ([]MetricSeries, error) {
	// Get value from Prometheus.
	matrix, err := g.q.query(ctx, query, start, step)
	if err != nil {
		return []MetricSeries{}, err
	}

	return transformMatrix(matrix), nil
}

func (g *graph) sortSeries(allSeries []MetricSeries) []MetricSeries {
	// Sort.
	sort.Slice(allSeries, func(i, j int) bool {
		return allSeries[i].ID < allSeries[j].ID
	})

	return allSeries
}

// createIndexedSlices will create the slices required create a render.Series based on these slices
func (g *graph) createIndexedSlices(start, end time.Time, step time.Duration, capacity int) (xLabels []string, indexedTime []time.Time) {
	xLabels = make([]string, capacity)
	indexedTime = make([]time.Time, capacity)

	format := TimeRangeTimeStringFormat(end.Sub(start), capacity)
	for i := 0; i < capacity; i++ {
		t := start.Add(time.Duration(i) * step).Local()
		xLabels[i] = t.Format(format)
		indexedTime[i] = t
	}

	return xLabels, indexedTime
}

// TimeRangeTimeStringFormat returns the best visual string format for a
// time range.
// TODO(slok): Use better the steps to get more accurate formats.
func TimeRangeTimeStringFormat(timeRange time.Duration, steps int) string {
	const (
		hourMinuteSeconds  = "15:04:05"
		hourMinute         = "15:04"
		monthDayHourMinute = "01/02 15:04"
		monthDay           = "01/02"
	)

	if steps == 0 {
		steps = 1
	}

	switch {
	// If greater than 15 day then always return month and day.
	case timeRange > 15*24*time.Hour:
		return monthDay
	// If greater than 1 day always return day and time.
	case timeRange > 24*time.Hour:
		return monthDayHourMinute
	// If always less than 1 minute return with seconds.
	case timeRange < time.Minute:
		return hourMinuteSeconds
	// If the minute based time has small duration steps we need to be
	// more accurate, so we use second base notation.
	case timeRange/time.Duration(steps) < 5*time.Second:
		return hourMinuteSeconds
	default:
		return hourMinute
	}
}

// Value is the value of a metric.
type Value float64

// Series are the series that can be rendered.
type Series struct {
	Label string
	Color string
	// XLabels are the labels that will be displayed on the X axis
	// the position of the label is the index of the slice.
	XLabels []string
	// Value slice, if there is no value we will use a nil value
	// we could use NaN floats but nil is more idiomatic and easy
	// to understand.
	Values []*Value
}

func (g *graph) transformToRenderable(series []MetricSeries, xLabels []string, indexedTime []time.Time) []Series {
	var renderSeries []Series

	var colorman widgetColorManager

	// Create the different series to render.
	for _, s := range series {
		// Init data.
		// This indexes will be used to query the different slices
		// into one single time based XY graph.
		values := make([]*Value, len(xLabels))
		timeIndex := 0
		metricIndex := 0
		valueIndex := 0

		// For every value/datapoint we will find where does it belong, to
		// do so we will check one by one each of the metrics if belongs
		// to a current time range, we do this checking if the metric timestamp
		// is after the current timestamp and before the next timestamp.
		for {
			if metricIndex >= len(s.Metrics) ||
				timeIndex >= len(indexedTime) ||
				valueIndex >= len(values) {
				break
			}

			m := s.Metrics[metricIndex]
			ts := indexedTime[timeIndex]

			// If metric is before the timestamp being processed in this
			// iteration then we don't need this metric (too late for it).
			if m.TS.Before(ts) {
				metricIndex++
				continue
			}

			// If we have a next Timestamp then check if the current TS
			// is before the next TS, if not then this metric doesn't
			// belong to this iteration, and belong to a future one.
			if timeIndex < len(indexedTime)-1 {
				nextTS := indexedTime[timeIndex+1]
				// If after means we should ignore this range, so we
				// check the null policy in case we need to fill the
				// empty datapoint space.
				if m.TS.After(nextTS) {
					// The null point mode setting is used to fill the gaps in the values,
					// sometimes the graph has N datapoints and we don't have enough datapoints
					// to create a good renderable graph. This way we can fill this gaps and make
					// the graph renderable.
					v := Value(m.Value)
					values[valueIndex] = &v

					timeIndex++
					valueIndex++
					continue
				}
			}
			// This value belongs here.
			v := Value(m.Value)
			values[valueIndex] = &v
			valueIndex++
			metricIndex++
			timeIndex++
		}
		// Create the renderable series.
		renderSeries = append(renderSeries, Series{
			Label:   s.ID,
			Color:   colorman.GetColorFromSeriesLegend(),
			XLabels: xLabels,
			Values:  values,
		})
	}

	return renderSeries
}

// transformMatrix will get a prometheus Matrix and transform to a domain model
// MetricSeries slice.
// A Prometheus Matrix is a slices of metrics (group of labels) that have multiple
// samples (in a slice of samples).
func transformMatrix(matrix promql.Matrix) []MetricSeries {
	var res []MetricSeries

	// Use a map to index the different series based on labels.
	for _, s := range matrix {
		labels := labelSetToMap(s.Metric)
		series := MetricSeries{
			ID:     s.Metric.String(),
			Labels: sanitizeLabels(labels),
		}

		// Add the metric to the series.
		for _, f := range s.Floats {
			series.Metrics = append(series.Metrics, Metric{
				TS:    time.Unix(0, f.T*int64(time.Millisecond)),
				Value: f.F,
			})
		}

		res = append(res, series)
	}

	return res
}

func labelSetToMap(ls labels.Labels) map[string]string {
	res := map[string]string{}
	for _, v := range ls {
		res[v.Name] = v.Value
	}

	return res
}

// sanitizeLabels will sanitize the map label values.
//   - Remove special labels if required (start with `__`).
func sanitizeLabels(m map[string]string) map[string]string {
	res := map[string]string{}
	for k, v := range m {
		if strings.HasPrefix(k, "__") {
			continue
		}
		res[k] = v
	}

	return res
}

func (g *graph) getWindowCapacity() int {
	// Sometimes the widget is not ready to return the capacity of the window, so we try the
	// best effort by trying multiple times with a small sleep so if we are lucky we can get
	// on one of the retries, and we don't need to wait for a full sync1 iteration (e.g. 10s),
	// this is not common but happens almost when creating the widgets for the first time.
	capacity := 0
	for i := 0; i < graphPointQuantityRetries; i++ {
		capacity = g.widgetGraph.ValueCapacity()
		if capacity != 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return capacity
}
