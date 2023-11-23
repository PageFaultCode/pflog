package pflog

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

const (
	testBadLevel           = 32
	testBacklogDepth       = 500
	testLowBacklogDepth    = 10
	testBadBacklogDepth    = -1
	testBadLevelConversion = 42
	testBacklogStart       = 0
	testBacklogNext        = 10
	testBacklogWrapStart   = 1
	testBacklogWrapNext    = 1
	testBacklogNextAfter2  = 2
	testBacklogNextAfter3  = 4
	testTagCount           = 3
)

var (
	one   = 1
	two   = 2
	three = 3
)

type LogTestSuite struct {
	suite.Suite
}

func (suite *LogTestSuite) TestLevels() {
	log := New()

	suite.Nil(log.SetLevel(Trace))
	suite.Nil(log.SetLevel(Error))
	suite.NotNil(log.SetTriggerLevel(testBadLevel))
	suite.NotNil(log.SetTriggerLevel(Trace))
	suite.Nil(log.SetTriggerLevel(Fatal))
}

func (suite *LogTestSuite) TestSetBacklogDepth() {
	log := New()

	suite.Nil(log.SetBacklogDepth(testBacklogDepth))
	suite.NotNil(log.SetBacklogDepth(testBadBacklogDepth))
}

func (suite *LogTestSuite) TestCompactDuplicates() {
	log := New()

	suite.Assert().Equal(log.GetCompactDuplicates(), true)
	log.SetCompactDuplicates(false)
	suite.Assert().Equal(log.GetCompactDuplicates(), false)
}

//revive:disable
func (suite *LogTestSuite) TestLog() {
	log := New()

	_ = log.AddOutputTarget(os.Stdout)

	log.Log(Trace, "testing")
	log.Logf(Trace, "testing: %d %d %d", 1, 2, 3)
	suite.Assert().Equal(testBacklogStart, log.firstEntry)
	suite.Assert().Equal(testBacklogNextAfter2, log.nextEntry)

	// add some duplicates
	log.Logf(Trace, "testing: %d %d %d", 1, 2, 3)
	log.Logf(Trace, "testing: %d %d %d", 1, 2, 3)

	suite.Assert().Equal(testBacklogStart, log.firstEntry)
	suite.Assert().Equal(testBacklogNextAfter2, log.nextEntry)

	// adds the duplicate entry and the next one
	// moving ahead 2
	log.Log(Trace, "something else")
	suite.Assert().Equal(testBacklogStart, log.firstEntry)
	suite.Assert().Equal(testBacklogNextAfter3, log.nextEntry)

	log.Log(Fatal, "this is so bad!")

	// Fatal will trigger a buffer dump
	suite.Assert().Equal(testBacklogStart, log.firstEntry)
	suite.Assert().Equal(0, log.nextEntry)
}

//revive:enable

func (suite *LogTestSuite) TestBacklogOverflow() {
	log := New()

	_ = log.AddOutputTarget(os.Stdout)

	err := log.SetBacklogDepth(testLowBacklogDepth)
	suite.Assert().Nil(err)

	log.Log(Information, "testing1")
	log.Log(Information, "testing2")
	log.Log(Information, "testing3")
	log.Log(Information, "testing4")
	log.Log(Information, "testing5")
	log.Log(Information, "testing6")
	log.Log(Information, "testing7")
	log.Log(Information, "testing8")
	log.Log(Information, "testing9")
	log.Log(Information, "testing10")
	suite.Assert().Equal(testBacklogStart, log.firstEntry)
	suite.Assert().Equal(testBacklogNext, log.nextEntry)

	// add two seconds so logs are shown to be in order
	time.Sleep(time.Second * 2)
	log.Log(Information, "testing11")
	suite.Assert().Equal(testBacklogWrapStart, log.firstEntry)
	suite.Assert().Equal(testBacklogWrapNext, log.nextEntry)

	// do this last as it will affect the backlog
	log.dumpBuffer()
}
func (suite *LogTestSuite) TestConvertLevelToString() {
	level, err := convertLevelToString(Trace, true)
	suite.Nil(err)

	suite.Assert().Equal(level, "TRACE")

	_, err = convertLevelToString(testBadLevelConversion, true)
	suite.NotNil(err)

	level, err = convertLevelToString(Error, false)
	suite.Nil(err)
	suite.Assert().Equal(level, "error")
}

func (suite *LogTestSuite) TestAddOutputTarget() {
	log := New()

	index := log.AddOutputTarget(os.Stdout)

	suite.Assert().Equal(0, index)

	log.Log(Error, "testing")
}

func (suite *LogTestSuite) TestClone() {
	log := New()

	_ = log.AddOutputTarget(os.Stdout)

	err := log.SetBacklogDepth(testLowBacklogDepth)
	suite.Assert().Nil(err)

	log.AddTag("one", one)
	log.AddTag("two", two)
	log.AddTag("three", three)

	cloneLog := log.Clone()

	suite.Assert().Equal(cloneLog.backlogDepth, log.backlogDepth)
	suite.Assert().Equal(cloneLog.compactDuplicates, log.compactDuplicates)
	suite.Assert().Equal(cloneLog.level, log.level)
	suite.Assert().Equal(cloneLog.triggerLevel, log.triggerLevel)

	suite.Assert().Equal(len(cloneLog.tags), testTagCount)
}

func (suite *LogTestSuite) TestBufferClear() {
	log := New()

	_ = log.AddOutputTarget(os.Stdout)

	err := log.SetBacklogDepth(testBacklogDepth)
	suite.Assert().Nil(err)

	suite.Nil(log.SetTriggerLevel(Error))

	// Should trigger and dump all
	log.Log(Error, "testing1")

	// Should also trigger and dump all
	log.Log(Error, "testing2")
	suite.Assert().Equal(0, log.firstEntry)
	suite.Assert().Equal(0, log.nextEntry)
}

func TestLoggingTestSuite(t *testing.T) {
	suite.Run(t, new(LogTestSuite))
}
