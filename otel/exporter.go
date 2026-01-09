// Package otel provides OpenTelemetry metrics export functionality for git-sizer.
// It sends repository size metrics to Datadog via OTLP.
package otel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"

	"github.com/github/git-sizer/sizes"
)

type Config struct {
	Endpoint       string
	Headers        map[string]string
	RepositoryName string
	Insecure       bool // disables TLS for the connection.
}

type Metric struct {
	Name           string
	Description    string
	Value          uint64
	Unit           string
	LevelOfConcern float64
}

func ExportMetrics(ctx context.Context, cfg Config, historySize *sizes.HistorySize) error {
	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(cfg.Endpoint),
	}

	if cfg.Insecure {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	if len(cfg.Headers) > 0 {
		opts = append(opts, otlpmetrichttp.WithHeaders(cfg.Headers))
	}

	exporter, err := otlpmetrichttp.New(ctx, opts...)
	if err != nil {
		return fmt.Errorf("creating OTLP exporter: %w", err)
	}
	defer exporter.Shutdown(ctx)

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("git-sizer"),
		attribute.String("repository", cfg.RepositoryName),
	)

	commonAttrs := []attribute.KeyValue{
		attribute.String("service", "git-sizer"),
		attribute.String("repository", cfg.RepositoryName),
	}

	metrics := collectMetrics(historySize)

	now := time.Now()
	scopeMetrics := buildScopeMetrics(metrics, commonAttrs, now)

	resourceMetrics := &metricdata.ResourceMetrics{
		Resource:     res,
		ScopeMetrics: scopeMetrics,
	}

	if err := exporter.Export(ctx, resourceMetrics); err != nil {
		return fmt.Errorf("exporting metrics: %w", err)
	}

	return nil
}

func buildScopeMetrics(metrics []Metric, commonAttrs []attribute.KeyValue, now time.Time) []metricdata.ScopeMetrics {
	var metricData []metricdata.Metrics

	for _, m := range metrics {
		metricName := fmt.Sprintf("git_sizer.%s", m.Name)

		attrs := make([]attribute.KeyValue, len(commonAttrs))
		copy(attrs, commonAttrs)

		concernLevel := m.LevelOfConcern
		if concernLevel > 30 {
			concernLevel = 50
		}
		attrs = append(attrs,
			attribute.Float64("concern_level", concernLevel),
		)

		attrSet := attribute.NewSet(attrs...)

		dataPoint := metricdata.DataPoint[int64]{
			Attributes: attrSet,
			StartTime:  now,
			Time:       now,
			Value:      int64(m.Value),
		}

		gaugeData := metricdata.Gauge[int64]{
			DataPoints: []metricdata.DataPoint[int64]{dataPoint},
		}

		metricData = append(metricData, metricdata.Metrics{
			Name:        metricName,
			Description: m.Description,
			Unit:        m.Unit,
			Data:        gaugeData,
		})
	}

	return []metricdata.ScopeMetrics{
		{
			Scope: instrumentation.Scope{
				Name:    "git-sizer",
				Version: "1.0.0",
			},
			Metrics: metricData,
		},
	}
}

func collectMetrics(h *sizes.HistorySize) []Metric {
	metrics := []Metric{
		// Overall repository size - Commits
		{
			Name:           "unique_commit_count",
			Description:    "The total number of distinct commit objects",
			Value:          uint64Val(h.UniqueCommitCount),
			Unit:           "{commits}",
			LevelOfConcern: float64(uint64Val(h.UniqueCommitCount)) / 500e3,
		},
		{
			Name:           "unique_commit_size",
			Description:    "The total size of all commit objects",
			Value:          uint64Val(h.UniqueCommitSize),
			Unit:           "By",
			LevelOfConcern: float64(uint64Val(h.UniqueCommitSize)) / 250e6,
		},

		// Trees
		{
			Name:           "unique_tree_count",
			Description:    "The total number of distinct tree objects",
			Value:          uint64Val(h.UniqueTreeCount),
			Unit:           "{trees}",
			LevelOfConcern: float64(uint64Val(h.UniqueTreeCount)) / 1.5e6,
		},
		{
			Name:           "unique_tree_size",
			Description:    "The total size of all distinct tree objects",
			Value:          uint64Val(h.UniqueTreeSize),
			Unit:           "By",
			LevelOfConcern: float64(uint64Val(h.UniqueTreeSize)) / 2e9,
		},
		{
			Name:           "unique_tree_entries",
			Description:    "The total number of entries in all distinct tree objects",
			Value:          uint64Val(h.UniqueTreeEntries),
			Unit:           "{entries}",
			LevelOfConcern: float64(uint64Val(h.UniqueTreeEntries)) / 50e6,
		},

		// Blobs
		{
			Name:           "unique_blob_count",
			Description:    "The total number of distinct blob objects",
			Value:          uint64Val(h.UniqueBlobCount),
			Unit:           "{blobs}",
			LevelOfConcern: float64(uint64Val(h.UniqueBlobCount)) / 1.5e6,
		},
		{
			Name:           "unique_blob_size",
			Description:    "The total size of all distinct blob objects",
			Value:          uint64Val(h.UniqueBlobSize),
			Unit:           "By",
			LevelOfConcern: float64(uint64Val(h.UniqueBlobSize)) / 10e9,
		},

		// Tags
		{
			Name:           "unique_tag_count",
			Description:    "The total number of annotated tags",
			Value:          uint64Val(h.UniqueTagCount),
			Unit:           "{tags}",
			LevelOfConcern: float64(uint64Val(h.UniqueTagCount)) / 25e3,
		},

		// References
		{
			Name:           "reference_count",
			Description:    "The total number of references",
			Value:          uint64Val(h.ReferenceCount),
			Unit:           "{refs}",
			LevelOfConcern: float64(uint64Val(h.ReferenceCount)) / 25e3,
		},

		// Biggest objects - Commits
		{
			Name:           "max_commit_size",
			Description:    "The size of the largest single commit",
			Value:          uint64Val(h.MaxCommitSize),
			Unit:           "By",
			LevelOfConcern: float64(uint64Val(h.MaxCommitSize)) / 50e3,
		},
		{
			Name:           "max_parent_count",
			Description:    "The most parents of any single commit",
			Value:          uint64Val(h.MaxParentCount),
			Unit:           "{parents}",
			LevelOfConcern: float64(uint64Val(h.MaxParentCount)) / 10,
		},

		// Biggest objects - Trees
		{
			Name:           "max_tree_entries",
			Description:    "The most entries in any single tree",
			Value:          uint64Val(h.MaxTreeEntries),
			Unit:           "{entries}",
			LevelOfConcern: float64(uint64Val(h.MaxTreeEntries)) / 1000,
		},

		// Biggest objects - Blobs
		{
			Name:           "max_blob_size",
			Description:    "The size of the largest blob object",
			Value:          uint64Val(h.MaxBlobSize),
			Unit:           "By",
			LevelOfConcern: float64(uint64Val(h.MaxBlobSize)) / 10e6,
		},

		// History structure
		{
			Name:           "max_history_depth",
			Description:    "The longest chain of commits in history",
			Value:          uint64Val(h.MaxHistoryDepth),
			Unit:           "{commits}",
			LevelOfConcern: float64(uint64Val(h.MaxHistoryDepth)) / 500e3,
		},
		{
			Name:           "max_tag_depth",
			Description:    "The longest chain of annotated tags pointing at one another",
			Value:          uint64Val(h.MaxTagDepth),
			Unit:           "{tags}",
			LevelOfConcern: float64(uint64Val(h.MaxTagDepth)) / 1.001,
		},

		// Biggest checkouts
		{
			Name:           "max_expanded_tree_count",
			Description:    "The number of directories in the largest checkout",
			Value:          uint64Val(h.MaxExpandedTreeCount),
			Unit:           "{directories}",
			LevelOfConcern: float64(uint64Val(h.MaxExpandedTreeCount)) / 2000,
		},
		{
			Name:           "max_path_depth",
			Description:    "The maximum path depth in any checkout",
			Value:          uint64Val(h.MaxPathDepth),
			Unit:           "{levels}",
			LevelOfConcern: float64(uint64Val(h.MaxPathDepth)) / 10,
		},
		{
			Name:           "max_path_length",
			Description:    "The maximum path length in any checkout",
			Value:          uint64Val(h.MaxPathLength),
			Unit:           "By",
			LevelOfConcern: float64(uint64Val(h.MaxPathLength)) / 100,
		},
		{
			Name:           "max_expanded_blob_count",
			Description:    "The maximum number of files in any checkout",
			Value:          uint64Val(h.MaxExpandedBlobCount),
			Unit:           "{files}",
			LevelOfConcern: float64(uint64Val(h.MaxExpandedBlobCount)) / 50e3,
		},
		{
			Name:           "max_expanded_blob_size",
			Description:    "The maximum sum of file sizes in any checkout",
			Value:          uint64Val(h.MaxExpandedBlobSize),
			Unit:           "By",
			LevelOfConcern: float64(uint64Val(h.MaxExpandedBlobSize)) / 1e9,
		},
		{
			Name:           "max_expanded_link_count",
			Description:    "The maximum number of symlinks in any checkout",
			Value:          uint64Val(h.MaxExpandedLinkCount),
			Unit:           "{symlinks}",
			LevelOfConcern: float64(uint64Val(h.MaxExpandedLinkCount)) / 25e3,
		},
		{
			Name:           "max_expanded_submodule_count",
			Description:    "The maximum number of submodules in any checkout",
			Value:          uint64Val(h.MaxExpandedSubmoduleCount),
			Unit:           "{submodules}",
			LevelOfConcern: float64(uint64Val(h.MaxExpandedSubmoduleCount)) / 100,
		},
	}

	return metrics
}

// uint64Val is a helper to convert counts.Humanable types to uint64.
func uint64Val[T interface{ ToUint64() (uint64, bool) }](v T) uint64 {
	val, _ := v.ToUint64()
	return val
}
