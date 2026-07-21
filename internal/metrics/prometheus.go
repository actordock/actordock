// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// InstallPrometheus registers a Prometheus reader as the global MeterProvider
// and returns an HTTP handler for GET /metrics.
func InstallPrometheus() (http.Handler, error) {
	exporter, err := otelprometheus.New()
	if err != nil {
		return nil, fmt.Errorf("prometheus exporter: %w", err)
	}
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(provider)
	return promhttp.Handler(), nil
}
