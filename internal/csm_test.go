package internal

import (
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
)

func TestCachedSecretsManager_getByJmesPath(t *testing.T) {
	m := NewCachedSecretsManager()
	s, found := m.getByJmesPath("secretName0", `{"a": {"b": {"c": "value"}}}`, "a.b.c")
	assert.Equal(t, "value", s)
	assert.True(t, found)

	s, found = m.getByJmesPath("secretName1", `{"a": {"b": {"c": "value"}}}`, "a")
	assert.Equal(t, `{"b":{"c":"value"}}`, s)
	assert.True(t, found)

	s, found = m.getByJmesPath("secretName2", `{"a": {"b": {"c": 132}}}`, "a.b.c")
	assert.Equal(t, "132", s)
	assert.True(t, found)

	s, found = m.getByJmesPath("secretName3", `{"a": {"b": {"c": "value"}}}`, "m")
	assert.Equal(t, "", s)
	assert.False(t, found)

	s, found = m.getByJmesPath("secretName4", `{"a": {"b": {"c": 98.1}}}`, "a.b.c")
	s2, _ := strconv.ParseFloat(s, 8)
	assert.Equal(t, 98.1, s2)
	assert.True(t, found)
}

func TestCachedSecretsManager_getByJmesPathCaching(t *testing.T) {
	m := NewCachedSecretsManager()
	s, found := m.getByJmesPath("secretName", `{"a": {"b": {"c": "value"}}}`, "a.b.c")
	assert.Equal(t, "value", s)
	assert.True(t, found)

	s, found = m.getByJmesPath("secretName", `{"a": {"b": {"c": "newvalue"}}}`, "a.b.c")
	assert.Equal(t, "value", s)
	assert.True(t, found)

}
