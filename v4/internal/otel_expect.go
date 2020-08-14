package internal

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/api/kv"
	"go.opentelemetry.io/otel/api/trace/testtrace"
)

// OpenTelemetryExpect implements internal.Expect for use in testing.
type OpenTelemetryExpect struct {
	Spans *testtrace.StandardSpanRecorder
}

func spansMatch(want WantSpan, span *testtrace.Span) error {
	name := span.Name()
	if want.Name != "" {
		if name != want.Name {
			return fmt.Errorf("Incorrect span name:\n\texpect=%s actual=%s",
				want.Name, name)
		}
	}
	spanCtx := span.SpanContext()
	if want.SpanID != "" {
		if id := spanCtx.SpanID.String(); id != want.SpanID {
			return fmt.Errorf("Incorrect id for span '%s':\n\texpect=%s actual=%s",
				name, want.SpanID, id)
		}
	}
	if want.TraceID != "" {
		if id := spanCtx.TraceID.String(); id != want.TraceID {
			return fmt.Errorf("Incorrect trace id for span '%s':\n\texpect=%s actual=%s",
				name, want.TraceID, id)
		}
	}
	if want.ParentID != "" {
		id := span.ParentSpanID().String()
		if want.ParentID == MatchAnyParent {
			if id == MatchNoParent {
				return fmt.Errorf("Incorrect parent id for span '%s': expected a parent but found none",
					name)
			}
		} else if id != want.ParentID {
			return fmt.Errorf("Incorrect parent id for span '%s':\n\texpect=%s actual=%s",
				name, want.ParentID, id)
		}
	}
	if want.Kind != "" {
		if kind := span.SpanKind().String(); kind != want.Kind {
			return fmt.Errorf("Incorrect kind for span '%s':\n\texpect=%s actual=%s",
				name, want.Kind, kind)
		}
	}
	if !want.SkipAttrsTest && want.Attributes != nil {
		foundAttrs := span.Attributes()
		if len(foundAttrs) != len(want.Attributes) {
			return fmt.Errorf("Incorrect number of attributes for span '%s':\n\texpect=%d actual=%d",
				name, len(want.Attributes), len(foundAttrs))
		}
		for k, v := range want.Attributes {
			if foundVal, ok := foundAttrs[kv.Key(k)]; ok {
				if f := foundVal.AsInterface(); v != MatchAnything && f != v {
					return fmt.Errorf("Incorrect value for attr '%s' on span '%s':\n\texpect=%s actual=%s",
						k, name, v, f)
				}
			} else {
				return fmt.Errorf("Attr '%s' not found on span '%s'", k, name)
			}
		}
	}
	if code := span.StatusCode(); want.StatusCode != code {
		return fmt.Errorf("Incorrect status code for span '%s':\n\texpect=%d actual=%d",
			name, want.StatusCode, code)
	}
	return nil
}

func (e *OpenTelemetryExpect) spans() []*testtrace.Span {
	return e.Spans.Completed()
}

// ExpectSpanEvents TODO
func (e *OpenTelemetryExpect) ExpectSpanEvents(t Validator, want []WantSpan) {
	t.Helper()
	spans := e.spans()
	if len(want) != len(spans) {
		t.Errorf("Incorrect number of recorded spans: expect=%d actual=%d",
			len(want), len(spans))
		return
	}
	for i := 0; i < len(want); i++ {
		if err := spansMatch(want[i], spans[i]); err != nil {
			t.Error(err)
		}
	}
}

func (e *OpenTelemetryExpect) expectSpanPresent(t Validator, want WantSpan) {
	t.Helper()
	for _, span := range e.spans() {
		if err := spansMatch(want, span); err == nil {
			return
		}
	}
	t.Errorf("Span '%s' not found", want.Name)
}

func (e *OpenTelemetryExpect) expectSpanPayloadReceived(t Validator) {
	t.Helper()
	for _, span := range e.spans() {
		if span.ParentSpanID().String() == MatchNoParent {
			t.Errorf("Span '%s' expected ParentID but found none", span.Name())
		}
	}
}

// ExpectCustomEvents TODO
func (e *OpenTelemetryExpect) ExpectCustomEvents(t Validator, want []WantEvent) {}

// ExpectErrors TODO
func (e *OpenTelemetryExpect) ExpectErrors(t Validator, want []WantError) {}

// ExpectErrorEvents TODO
func (e *OpenTelemetryExpect) ExpectErrorEvents(t Validator, want []WantEvent) {}

// ExpectMetrics TODO
func (e *OpenTelemetryExpect) ExpectMetrics(t Validator, want []WantMetric) {
	t.Helper()
	for _, metric := range want {
		if strings.HasPrefix(metric.Name, "WebTransaction/Go/") {
			name := strings.TrimPrefix(metric.Name, "WebTransaction/Go/")
			if strings.HasPrefix(name, "Message/") {
				if split := strings.SplitN(name, "/", 5); len(split) == 5 {
					name = split[4] + " receive"
				}
			}
			span := WantSpan{
				Name: name,
			}
			e.expectSpanPresent(t, span)
		}
		if strings.HasPrefix(metric.Name, "OtherTransaction/Go/") {
			name := strings.TrimPrefix(metric.Name, "OtherTransaction/Go/")
			if strings.HasPrefix(name, "Message/") {
				if split := strings.SplitN(name, "/", 5); len(split) == 5 {
					name = split[4] + " receive"
				}
			}
			span := WantSpan{
				Name: name,
			}
			e.expectSpanPresent(t, span)
		}
		if strings.HasPrefix(metric.Name, "External/") {
			if split := strings.SplitN(metric.Name, "/", 4); len(split) == 4 {
				name := split[2] + " " + split[3] + " " + split[1]
				span := WantSpan{
					Name:     name,
					ParentID: MatchAnyParent,
				}
				e.expectSpanPresent(t, span)
			}
		}
		if strings.HasPrefix(metric.Name, "Datastore/statement/") && metric.Scope == "" {
			if split := strings.SplitN(metric.Name, "/", 5); len(split) == 5 {
				name := fmt.Sprintf("'%s' on '%s' using '%s'", split[4], split[3], split[2])
				span := WantSpan{
					Name:     name,
					ParentID: MatchAnyParent,
				}
				e.expectSpanPresent(t, span)
			}
		}
		if strings.HasPrefix(metric.Name, "Datastore/operation/") {
			span := WantSpan{
				// Since we do not know the table name we can not infer the
				// span name.
				Name:     "",
				ParentID: MatchAnyParent,
			}
			e.expectSpanPresent(t, span)
		}
		if strings.HasPrefix(metric.Name, "Custom/") && metric.Scope == "" {
			if split := strings.SplitN(metric.Name, "/", 2); len(split) == 2 {
				span := WantSpan{
					Name:     split[1],
					ParentID: MatchAnyParent,
				}
				e.expectSpanPresent(t, span)
			}
		}
		if strings.HasPrefix(metric.Name, "TransportDuration") &&
			strings.HasSuffix(metric.Name, "/all") {
			// The presence of this metric is used to test that a
			// distributed trace payload is successfully received.
			e.expectSpanPayloadReceived(t)
		}
		if strings.HasPrefix(metric.Name, "MessageBroker") && metric.Scope == "" {
			if split := strings.SplitN(metric.Name, "/", 6); len(split) == 6 {
				name := split[5] + " send"
				span := WantSpan{
					Name:     name,
					ParentID: MatchAnyParent,
				}
				e.expectSpanPresent(t, span)
			}
		}
	}
}

// ExpectMetricsPresent TODO
func (e *OpenTelemetryExpect) ExpectMetricsPresent(t Validator, want []WantMetric) {
	e.ExpectMetrics(t, want)
}

// ExpectTxnMetrics TODO
func (e *OpenTelemetryExpect) ExpectTxnMetrics(t Validator, want WantTxn) {
	t.Helper()
	spans := e.spans()
	if len(spans) == 0 {
		t.Error("No spans recorded")
		return
	}
	exp := WantSpan{
		Name:     want.Name,
		ParentID: MatchNoParent,
	}
	if err := spansMatch(exp, spans[0]); err != nil {
		t.Error(err)
	}
}

// ExpectTxnTraces TODO
func (e *OpenTelemetryExpect) ExpectTxnTraces(t Validator, want []WantTxnTrace) {}

// ExpectSlowQueries TODO
func (e *OpenTelemetryExpect) ExpectSlowQueries(t Validator, want []WantSlowQuery) {}
