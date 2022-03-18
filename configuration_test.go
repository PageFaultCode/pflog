// Package pflog defines all of the pflog package
package pflog

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

const testBacklog = 500

type ConfigurationTestSuite struct {
	suite.Suite
}

func (suite *ConfigurationTestSuite) TestSettingsLoad() {
	var configuration Configuration

	err := configuration.LoadConfigurationFile("settings.yaml")
	suite.Assert().Nil(err)

	suite.Assert().Equal(LogLevel(Information), convertStringToLevel(configuration.Settings.Level))
	suite.Assert().Equal(LogLevel(Error), convertStringToLevel(configuration.Settings.TriggerLevel))
	suite.Assert().Equal(testBacklog, configuration.Settings.Backlog)
}

func TestConfigurationTestSuite(t *testing.T) {
	suite.Run(t, new(ConfigurationTestSuite))
}
