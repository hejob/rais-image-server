// This file is an example of integrating an external APM system (DataDog)
// which needs to be able to do some setup, wrap all handlers, and do teardown
// when RAIS is shutting down.  Use of this plugin should be fairly
// straightforward, but you will have to add some configuration for DataDog.
//
// First, you must set up a DataDog agent.  If you do this with docker-compose,
// DD_API_KEY should be added to your .env, but you should be able to use our
// demo docker-compose.yml file otherwise.
//
// Then, "DatadogAddress" must be added to your rais.toml or else
// RAIS_DATADOGADDRESS must be in your RAIS environment.  This is shown in our
// demo docker-compose.yml.
//
// If you want instrumentation that goes deeper than request round-tripping,
// please be aware that RAIS does not currently support this.

package main

import (
	"net/http"

	"github.com/spf13/viper"
	"github.com/uoregon-libraries/gopkg/logger"
	httptrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/net/http"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var l *logger.Logger

// Initialize reads configuration and sets up the datadog agent
func Initialize() {
	var ddaddr = viper.GetString("DatadogAddress")

	if ddaddr == "" {
		l.Fatalf("ERROR: DatadogAddress must be configured, or RAIS_DATADOGADDRESS must be set in the environment")
	}

	l.Debugf("Connecting to datadog agent at %q", ddaddr)
	tracer.Start(tracer.WithAgentAddr(ddaddr))
}

// WrapHandler takes all RAIS routes' handlers and puts the datadog
// instrumentation into them
func WrapHandler(pattern string, handler http.Handler) (http.Handler, error) {
	return httptrace.WrapHandler(handler, "RAIS/datadog", pattern), nil
}

// Teardown tells datadog to shut down the tracer gracefully
func Teardown() {
	tracer.Stop()
}

// SetLogger is called by the RAIS server's plugin manager to let plugins use
// the central logger
func SetLogger(raisLogger *logger.Logger) {
	l = raisLogger
}
