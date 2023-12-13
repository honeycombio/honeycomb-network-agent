#!/usr/bin/env bats

load test_helpers/utilities

SCOPE="hny-network-agent"

@test "Agent includes service.name in resource attributes" {
  result=$(resource_attributes_received | jq "select(.key == \"service.name\").value.stringValue")
  assert_equal "$result" '"hny-network-agent"'
}

@test "Agent span includes custom resource attributes" {
  result=$(resource_attributes_received | jq "select(.key == \"environment\").value.stringValue")
  assert_equal "$result" '"smokey"'
}

@test "Agent includes Honeycomb agent resource attributes" {
  result=$(resource_attributes_received | jq "select(.key == \"honeycomb.agent.name\").value.stringValue")
  assert_equal "$result" '"Honeycomb Network Agent"'

  version=$(resource_attributes_received | jq "select(.key == \"honeycomb.agent.version\").value.stringValue")
  assert_not_empty "$version"
}

@test "Agent emits a span name '{http.method}' (per semconv)" {
  result=$(span_names_for ${SCOPE})
  assert_equal "$result" '"POST"'
}

@test "Agent includes specified headers as attributes" {
  result=$(span_attributes_for ${SCOPE} | jq "select(.key == \"http.request.header.user_agent\").value.stringValue")
  assert_equal "$result" '"curl/8.5.0"'
}

@test "Agent includes k8s source attributes" {
  result=$(span_attributes_for ${SCOPE} | jq "select(.key == \"source.k8s.container.name\").value.stringValue")
  assert_equal "$result" '"smoke-curl"'
}

@test "Agent includes k8s destination attributes" {
  result=$(span_attributes_for ${SCOPE} | jq "select(.key == \"destination.k8s.container.name\").value.stringValue")
  assert_equal "$result" '"echoserver"'
}

@test "HTTP span includes http.method attribute" {
  result=$(span_attributes_for ${SCOPE} | jq "select(.key == \"http.method\").value.stringValue")
  assert_equal "$result" '"POST"'
}

@test "HTTP span includes http.target attribute" {
  result=$(span_attributes_for ${SCOPE} | jq "select(.key == \"http.target\").value.stringValue")
  assert_equal "$result" '"/"'
}

@test "HTTP span includes http.status_code attribute" {
  result=$(span_attributes_for ${SCOPE} | jq "select(.key == \"http.status_code\").value.intValue")
  assert_equal "$result" '"405"'
}

@test "HTTP span includes client error attribute with 4xx response" {
  result=$(span_attributes_for ${SCOPE} | jq "select(.key == \"error\").value.stringValue")
  assert_equal "$result" '"HTTP client error"'
}

@test "Trace ID present in all spans" {
  trace_id=$(spans_from_scope_named ${SCOPE} | jq ".traceId")
  assert_not_empty "$trace_id"
}

@test "Span ID present in all spans" {
  span_id=$(spans_from_scope_named ${SCOPE} | jq ".spanId")
  assert_not_empty "$span_id"
}
