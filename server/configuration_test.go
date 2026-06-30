package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigurationDefaults(t *testing.T) {
	c := &configuration{}
	assert.Equal(t, "gpt-5.5", c.model())
	assert.Equal(t, "EN", c.defaultLanguage())

	c = &configuration{Model: "gpt-5.5-mini", DefaultLanguage: "ru"}
	assert.Equal(t, "gpt-5.5-mini", c.model())
	assert.Equal(t, "RU", c.defaultLanguage())

	c = &configuration{DefaultLanguage: "zz"} // unsupported → EN
	assert.Equal(t, "EN", c.defaultLanguage())
}
