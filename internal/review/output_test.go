package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputParserParseVariants(t *testing.T) {
	parser := NewOutputParser()

	result, err := parser.Parse(`{"summary":"ok","vibeCheck":{"score":5,"verdict":"ok","flags":[]},"recommendations":[],"approve":true}`)
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Summary)

	result, err = parser.Parse("```json\n{\"summary\":\"wrapped\",\"vibeCheck\":{\"score\":5,\"verdict\":\"ok\",\"flags\":[]},\"recommendations\":[],\"approve\":true}\n```")
	require.NoError(t, err)
	assert.Equal(t, "wrapped", result.Summary)

	result, err = parser.Parse("prefix {\"summary\":\"embedded\",\"vibeCheck\":{\"score\":5,\"verdict\":\"ok\",\"flags\":[]},\"recommendations\":[],\"approve\":true} suffix")
	require.NoError(t, err)
	assert.Equal(t, "embedded", result.Summary)

	_, err = parser.Parse("not json")
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to parse AI response as JSON")
}

func TestParseJSONAndHelpers(t *testing.T) {
	parser := NewOutputParser()

	_, err := parser.parseJSON(`{"vibeCheck":{"score":5,"verdict":"ok","flags":[]},"recommendations":[],"approve":true}`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "missing required field: summary")

	assert.Equal(t, `{"a":1}`, stripCodeFences("```json\n{\"a\":1}\n```"))
	assert.Equal(t, `{"a":1}`, stripCodeFences("```\n{\"a\":1}\n```"))
	assert.Equal(t, "", extractJSON("no braces"))
	assert.Equal(t, `{"a":{"b":1}}`, extractJSON("xx {\"a\":{\"b\":1}} yy"))
	assert.Equal(t, "", extractJSON("{\"a\":1"))
}
