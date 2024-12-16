package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/onsi/gomega/gmeasure/table"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "report",
		Short: "report on a ginkgo json",
		Long:  `This is used to analyze benchmark results.`,
	}
	input   string
	db      string
	verbose bool
)

func init() {
	rootCmd.AddCommand(reportCmd)
	rootCmd.PersistentFlags().StringVarP(&input, "input", "i", "report.json", "input file")
	rootCmd.PersistentFlags().StringVarP(&db, "db", "d", "db/", "path to json file database dir")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		log.Fatal(err)
	}
}

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "report on a ginkgo json",
	Long:  `This is used to analyze benchmark results.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var (
			mfs    map[string]*dto.MetricFamily
			parser expfmt.TextParser
		)

		out, err := os.ReadFile(input)
		if err != nil {
			return err
		}

		mfs, err = parser.TextToMetricFamilies(bytes.NewBuffer(out))
		if err != nil {
			return err
		}

		if _, ok := mfs["controller_runtime_reconcile_total"]; !ok {
			return fmt.Errorf("controller_runtime_reconcile_total not found")
		}

		controllers := []string{"gitrepo"}
		res := map[string]float64{}
		extractFromMetricFamilies(res, controllers, mfs)

		fmt.Println(prettyPrint(res))

		return nil
	},
}

func extractFromMetricFamilies(res map[string]float64, controllers []string, mfs map[string]*dto.MetricFamily) {
	// controller_runtime_reconcile_total{controller="gitrepo",result="error"} 0
	// controller_runtime_reconcile_total{controller="gitrepo",result="requeue"} 71
	// controller_runtime_reconcile_total{controller="gitrepo",result="requeue_after"} 155
	// controller_runtime_reconcile_total{controller="gitrepo",result="success"} 267
	mf := mfs["controller_runtime_reconcile_total"]
	for _, m := range mf.Metric {
		l := m.GetLabel()
		for _, c := range controllers {
			if l[0].GetValue() == c {
				v := m.Counter.GetValue()
				n := l[1].GetValue()
				res[c+"-controller_runtime_reconcile_total-"+n] += v
			}
		}
	}

	// controller_runtime_reconcile_time_seconds_sum{controller="gitrepo"} 185.52245399500018
	mf = mfs["controller_runtime_reconcile_time_seconds"]
	incMetric(res, "controller_runtime_reconcile_time_seconds", controllers, *mf.Type, mf.Metric)

	mf = mfs["controller_runtime_active_workers"]
	incMetric(res, "controller_runtime_active_workers", controllers, *mf.Type, mf.Metric)

	mf = mfs["controller_runtime_max_concurrent_reconciles"]
	incMetric(res, "controller_runtime_max_concurrent_reconciles", controllers, *mf.Type, mf.Metric)

	mf = mfs["controller_runtime_reconcile_errors_total"]
	incMetric(res, "controller_runtime_reconcile_errors_total", controllers, *mf.Type, mf.Metric)

	// workqueue metrics per controller
	for _, m := range []string{"workqueue_adds_total", "workqueue_queue_duration_seconds", "workqueue_retries_total", "workqueue_work_duration_seconds"} {
		if mf, ok := mfs[m]; ok {
			incMetric(res, m, controllers, *mf.Type, mf.Metric)
		}
	}

	// metrics without a controller label
	for _, m := range mfs["rest_client_requests_total"].Metric {
		l := m.GetLabel()
		code := l[0].GetValue()
		method := l[2].GetValue()
		res["rest_client_requests-"+method+"-"+code] += m.Counter.GetValue()
	}

	for _, m := range mfs["go_gc_duration_seconds"].Metric {
		res["go_gc_duration_seconds"] += m.Summary.GetSampleSum()
	}

	for _, m := range mfs["process_cpu_seconds_total"].Metric {
		res["process_cpu_seconds_total"] += m.Counter.GetValue()
	}

}

func incMetric(res map[string]float64, name string, controllers []string, t dto.MetricType, metrics []*dto.Metric) {
	for _, m := range metrics {
		l := m.GetLabel()
		for _, c := range controllers {
			if l[0].GetValue() == c {
				var v float64

				switch t {
				case dto.MetricType_COUNTER:
					v += m.Counter.GetValue()
				case dto.MetricType_GAUGE:
					v += m.Gauge.GetValue()
				case dto.MetricType_SUMMARY:
					v += m.Summary.GetSampleSum()
				case dto.MetricType_HISTOGRAM:
					v += m.Histogram.GetSampleSum()
					fmt.Println(c + " " + name)
					t := newHistTable(m.Histogram)
					fmt.Println(t.Render())
				}

				// per controller
				res[c+"-"+name] = v
				// add to total
				res["total-"+name] += v
			}
		}
	}
}

func newHistTable(h *dto.Histogram) *table.Table {
	t := table.NewTable()
	t.AppendRow(table.R(
		table.C("Count"),
		table.C("Upper Bound"),
		table.Divider("="),
	))

	// keys := slices.Sorted(maps.Keys(measurements))
	for _, b := range h.Bucket {
		t.AppendRow(table.R(
			table.C(fmt.Sprintf("%d", b.GetCumulativeCount())),
			table.C(fmt.Sprintf("%.08f", b.GetUpperBound())),
		))

	}
	return t
}

func prettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}
