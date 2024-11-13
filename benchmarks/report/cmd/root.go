package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/stat"

	"github.com/onsi/ginkgo/v2/types"
	gm "github.com/onsi/gomega/gmeasure"
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

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(reportCmd)
	rootCmd.PersistentFlags().StringVarP(&input, "input", "i", "report.json", "input file")
	rootCmd.PersistentFlags().StringVarP(&db, "db", "d", "db/", "path to json file database dir")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
}

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "report on a ginkgo json",
	Long:  `This is used to analyze benchmark results.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		population, err := loadDB(db)
		if err != nil {
			return err
		}
		// fmt.Print(prettyPrint(&population))

		sample, err := loadSampleFile(input)
		if err != nil {
			return err
		}
		fmt.Println("# Description of Setup")
		fmt.Println(sample.Description)
		fmt.Println()

		dsPop := Dataset{}
		for _, sample := range population.Samples {
			transformDataset(dsPop, sample)
		}

		// foreach experiment in population, calculate mean and stddev
		fmt.Println("# Measurement Scores Per Experiment")
		for experiment, xp := range dsPop {
			fmt.Printf("## %s\n", experiment)
			for measurement, sg := range xp {
				mean, stddev := stat.MeanStdDev(sg.Values, nil)
				if math.IsNaN(stddev) || stddev == 0 {
					continue
				}

				sg.Mean = mean
				sg.StdDev = stddev
				dsPop[experiment][measurement] = sg

				if _, ok := sample.Experiments[experiment]; !ok {
					//fmt.Printf("missing experiment %s\n", name)
					continue
				}

				if _, ok := sample.Experiments[experiment].Measurements[measurement]; !ok {
					//fmt.Printf("missing measurement %s for experiments %s\n", measurement, name)
					continue
				}

				m := sample.Experiments[experiment].Measurements[measurement]
				xp := sample.Experiments[experiment]
				zscore := stat.StdScore(m, mean, stddev)
				xp.ZScores = append(xp.ZScores, zscore)
				xp.Weights = append(xp.Weights, weight(measurement))
				sample.Experiments[experiment] = xp

				fmt.Printf("* %s: %d/5\n", measurement, grade(zscore))
				//fmt.Printf("%v\n%s: mean=%f, stddev=%f\n%#v\n", sg, measurement, mean, stddev, dsPop[name][measurement])
			}
			fmt.Println()
		}

		fmt.Println("# Scores Per Experiment")
		for name, xp := range sample.Experiments {
			zscores := minMax(xp.ZScores, -1.5, 1.5)
			avg := stat.Mean(zscores, xp.Weights)
			grade := grade(avg)

			xp := sample.Experiments[name]
			xp.Grade = grade
			xp.ZScores = zscores
			sample.Experiments[name] = xp

			fmt.Printf("* %s: %d/5\n", name, grade)
		}
		fmt.Println()

		if verbose {
			fmt.Println("# Population from DB")
			fmt.Println("```")
			fmt.Println(prettyPrint(dsPop))
			fmt.Println("```")
			fmt.Println()
			fmt.Println("# Current Sample")
			fmt.Println("```")
			fmt.Println(prettyPrint(sample))
			fmt.Println("```")
			fmt.Println()
		}

		fmt.Println("# Total Score")
		grades := []float64{}
		for _, xp := range sample.Experiments {
			grades = append(grades, float64(xp.Grade))
		}

		avg := stat.Mean(grades, nil)
		fmt.Printf("* %.01f/5\n", avg)

		return nil
	},
}

func minMax(values []float64, a float64, b float64) []float64 {
	max := floats.Max(values)
	min := floats.Min(values)

	result := make([]float64, len(values))

	for i := range values {
		result[i] = a + (values[i]-min)/(max-min)*(b-a)
	}

	return result
}

// Some measurements have a higher volatility than others, or are duplicated.
//
// "CPU": 14.029999999999973,
// "GCDuration": 1.9185229570000004,
// "Mem": 4,
// "MemDuring": 4,
// "NetworkRX": 68288672,
// "NetworkTX": 30662826,
// "ReconcileErrors": 0,
// "ReconcileRequeue": 65,
// "ReconcileRequeueAfter": 462,
// "ReconcileSuccess": 2329,
// "ReconcileTime": 8153.420151420956,
// "ResourceCount": 300,
// "WorkqueueAdds": 2844,
// "WorkqueueQueueDuration": 3911.157310051014,
// "WorkqueueRetries": 527,
// "WorkqueueWorkDuration": 8169.425508522996
func weight(name string) float64 {
	switch name {
	case "GCDuration", "Mem", "MemDuring", "NetworkTX", "NetworkRX":
		return 0.1
	case "ReconcileErrors", "ReconcileRequeue", "ReconcileRequeueAfter", "ReconcileSuccess":
		return 0.2
	case "CPU", "WorkqueueAdds", "WorkqueueQueueDuration", "WorkqueueRetries", "WorkqueueWorkDuration":
		return 0.3
	case "ResourceCount":
		return 0.5
	case "ReconcileTime":
		return 1.0
	}

	return 1.0
}

// grade returns a grade from 1 to 5 based on zscore
// our measurements are better if they are below the average, i.e. lower
// NOTE need to adapt to range
func grade(zscore float64) int {
	if zscore > 1.5 {
		return 1 // bad
	}
	if zscore > 0.5 {
		return 2
	}
	if zscore > -0.5 {
		return 3
	}
	if zscore > -1.5 {
		return 4
	}
	return 5 // good
}

// transformDataset takes a sample and transforms it into a dataset
// dataset, output by experiment
//
//	{ "50-gitrepo": {
//	   "CPU": { "mean": 0.5, "stddev": 0.1, val: [0.4, 0.5, 0.6] },
//	   "GC": { "mean": 0.5, "stddev": 0.1, val: [0.4, 0.5, 0.6] },
//	  },
//	 "50-bundle": {
//	   "CPU": { "mean": 0.5, "stddev": 0.1, val: [0.4, 0.5, 0.6] },
//	   "GC": { "mean": 0.5, "stddev": 0.1, val: [0.4, 0.5, 0.6] },
//	  },
//	}
func transformDataset(ds Dataset, sample Sample) {
	for name, experiment := range sample.Experiments {
		for measurement, value := range experiment.Measurements {
			if _, ok := ds[name]; !ok {
				ds[name] = map[string]SubGrade{}
			}
			if _, ok := ds[name][measurement]; !ok {
				ds[name][measurement] = SubGrade{
					Values: []float64{},
				}
			}
			tmp := ds[name][measurement]
			tmp.Values = append(tmp.Values, value)
			ds[name][measurement] = tmp
		}
	}
}

type Dataset map[string]map[string]SubGrade

type SubGrade struct {
	Mean   float64
	StdDev float64
	ZScore float64
	Values []float64
}

func prettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}

// Population has data from all json reports
type Population struct {
	Samples []Sample
}

// Sample is a json report file
type Sample struct {
	Experiments map[string]Experiment
	Description string
}

// Experiment is a set of measurements, like from 50-gitrepo-1-bundle
// Measurements from the report are one dimensional, as most experiments don't
// use sampling
type Experiment struct {
	Measurements map[string]float64
	Weights      []float64

	ZScores []float64
	Grade   int
}

// jq '.[0].SpecReports.[].State' < b-2024-11-11_19:15:08.json
// jq '.[0].SpecReports.[].ReportEntries' < b-2024-11-12_15:25:33.json
// jq '.[0].SpecReports.[].ReportEntries.[0].Name' < b-2024-11-12_15:25:33.json
func loadDB(db string) (*Population, error) {
	// load the json files from db folder and parse them
	files, err := filepath.Glob(db + "/*.json")
	if err != nil {
		return nil, err
	}

	pop := &Population{}

	for _, file := range files {
		if s, err := loadSampleFile(file); err != nil {
			return nil, err
		} else if s != nil {
			pop.Samples = append(pop.Samples, *s)
		}
	}

	return pop, nil
}

func loadSampleFile(file string) (*Sample, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	reports := []types.Report{}
	err = json.Unmarshal(data, &reports)
	if err != nil {
		fmt.Printf("error: %s\n", data)
		return nil, err
	}
	if len(reports) < 1 {
		return nil, nil
	}

	report := (reports)[0]
	if len(report.SpecReports) < 1 {
		return nil, nil
	}
	if report.SpecReports[0].State != types.SpecStatePassed {
		return nil, nil
	}

	// fmt.Println("# File: " + file)
	s := Sample{
		Experiments: map[string]Experiment{},
	}

	for _, reports := range report.SpecReports {
		// fmt.Println("# Report: " + strings.Join(reports.ContainerHierarchyTexts, ""))
		if len(reports.ContainerHierarchyLabels) < 2 {
			for _, entry := range reports.ReportEntries {
				if entry.Name != "setup" {
					continue
				}

				xp := gm.Experiment{}
				data := entry.Value.AsJSON
				err = json.Unmarshal([]byte(data), &xp)
				if err != nil {
					fmt.Printf("error: %s\n", data)
					return nil, err
				}

				if xp.Name == "before" || xp.Name == "after" {
					for _, m := range xp.Measurements {
						switch m.Type {
						case gm.MeasurementTypeValue:
							if len(m.Values) < 1 {
								continue
							}
							v := fmt.Sprintf(m.PrecisionBundle.ValueFormat, m.Values[0])
							s.Description += fmt.Sprintf("* %s: %s\n", m.Name, v)

						case gm.MeasurementTypeDuration:
							if len(m.Durations) < 1 {
								continue
							}
							s.Description += fmt.Sprintf("* %s: %s\n", m.Name, m.Durations[0].Round(m.PrecisionBundle.Duration).String())
						case gm.MeasurementTypeNote:
							lines := strings.Split(strings.Trim(m.Note, "\n"), "\n")
							for i := range lines {
								s.Description += fmt.Sprintf("> %s\n", lines[i])
							}
						}
					}
					s.Description += "\n"
				}
			}
			continue
		}
		// fmt.Println("# Report: " + strings.Join(reports.ContainerHierarchyLabels[1], ""))

		// NOTE we need to normalize the measurements, eg. high is
		// better or low is better, calculate difference from
		// Before/After.
		for _, entry := range reports.ReportEntries {
			// fmt.Println("## Entry: " + entry.Name)

			e := Experiment{
				Measurements: map[string]float64{},
				// Label:        label, // = entry.Name
			}

			xp := gm.Experiment{}
			data := entry.Value.AsJSON
			err = json.Unmarshal([]byte(data), &xp)
			if err != nil {
				fmt.Printf("error: %s\n", data)
				return nil, err
			}

			for _, m := range xp.Measurements {
				if len(m.Values) < 1 {
					continue
				}
				if m.Type != gm.MeasurementTypeValue && m.Type != gm.MeasurementTypeDuration {
					continue
				}

				v := m.Values[0]

				// MemDuring is actually sampled, not a single value
				if m.Name == "MemDuring" {
					v = stat.Mean(m.Values, nil)
				}

				name := m.Name
				if strings.HasSuffix(m.Name, "Before") {
					name = strings.TrimSuffix(m.Name, "Before")
					v = -v
				} else {
					name = strings.TrimSuffix(m.Name, "After")
				}

				// fmt.Println("### Measurement: " + m.Name + " = " + name)
				e.Measurements[name] += v
			}
			s.Experiments[entry.Name] = e
		}
	}

	return &s, nil
}
