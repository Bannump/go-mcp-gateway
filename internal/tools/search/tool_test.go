package search_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Bannump/go-mcp-gateway/internal/tools/search"
)

type result struct {
	ID    string  `json:"id"`
	Title string  `json:"title"`
	Score float64 `json:"score"`
}

func TestSearchTool_TopK(t *testing.T) {
	tool := search.New()
	params, _ := json.Marshal(map[string]interface{}{"query": "Go goroutines", "top_k": 3})

	res, err := tool.Execute(context.Background(), params)
	require.NoError(t, err)
	assert.False(t, res.IsError)

	var results []result
	require.NoError(t, json.Unmarshal([]byte(res.Content[0].Text), &results))
	assert.LessOrEqual(t, len(results), 3)
}

func TestSearchTool_TopKCappedAt20(t *testing.T) {
	tool := search.New()
	params, _ := json.Marshal(map[string]interface{}{"query": "go", "top_k": 100})

	res, err := tool.Execute(context.Background(), params)
	require.NoError(t, err)
	assert.False(t, res.IsError)

	var results []result
	require.NoError(t, json.Unmarshal([]byte(res.Content[0].Text), &results))
	assert.LessOrEqual(t, len(results), 20)
}

func TestSearchTool_EmptyQuery(t *testing.T) {
	tool := search.New()
	params, _ := json.Marshal(map[string]interface{}{"query": ""})

	res, err := tool.Execute(context.Background(), params)
	require.NoError(t, err)
	assert.True(t, res.IsError)
}
