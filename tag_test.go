package pflog

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

const (
	tagTestInitialCount = 0
	tagTestFinalCount   = 3
)

var (
	tagOne             = 1
	tagTwo             = 2
	tagThree           = 3
	tagExpectedResults = "[FATAL] one: 1 two: 2 three: 3 tag testing"
)

type TagTestSuite struct {
	suite.Suite
}

func (suite *TagTestSuite) TestAddTag() {
	log := New()

	var buf bytes.Buffer

	_ = log.AddOutputTarget(&buf)

	suite.Assert().Equal(len(log.tags), tagTestInitialCount)

	log.AddTag("one", tagOne)
	log.AddTag("two", tagTwo)
	log.AddTag("three", tagThree)

	suite.Assert().Equal(len(log.tags), tagTestFinalCount)

	log.Log(Fatal, "tag testing")

	suite.Assert().True(strings.Contains(buf.String(), tagExpectedResults))
	print(buf.String())
}

func TestTagTestSuite(t *testing.T) {
	suite.Run(t, new(TagTestSuite))
}
