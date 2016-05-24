package memcached

import (
	"errors"
	"syscall"
	"testing"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/intelsdi-x/swan/pkg/executor/mocks"
	"github.com/intelsdi-x/swan/pkg/isolation"
	. "github.com/smartystreets/goconvey/convey"
)

func connectTimeoutSuccess(address string, timeout time.Duration) bool {
	return true
}

func connectTimeoutFailure(address string, timeout time.Duration) bool {
	return false
}

// TestMemcachedWithMockedExecutor runs a Memcached launcher with the mocked executor to simulate
// different cases like proper process execution and error case.
func TestMemcachedWithMockedExecutor(t *testing.T) {
	log.SetLevel(log.ErrorLevel)

	const (
		expectedCommand = "test -p 11211 -u memcached -t 4 -m 64 -c 1024"
		expectedHost    = "127.0.0.1"
	)
	Convey("When I create PID namespace isolation", t, func() {
		mockedExecutor := new(mocks.Executor)
		mockedTaskHandle := new(mocks.TaskHandle)
		var decorators []isolation.Decorator
		unshare, err := isolation.NewNamespace(syscall.CLONE_NEWPID)
		So(err, ShouldBeNil)
		decorators = append(decorators, unshare)

		Convey("While using Memcached launcher", func() {
			memcachedLauncher := New(
				mockedExecutor,
				DefaultMemcachedConfig("test"))
			memcachedLauncher.tryConnect = connectTimeoutSuccess
			Convey("While simulating proper execution", func() {
				mockedExecutor.On("Execute", expectedCommand).Return(mockedTaskHandle, nil).Once()
				mockedTaskHandle.On("Address").Return(expectedHost)

				Convey("Build command should create proper command", func() {
					command := memcachedLauncher.buildCommand()
					So(command, ShouldEqual, expectedCommand)

					Convey("Arguments passed to Executor should be a proper command", func() {
						task, err := memcachedLauncher.Launch()
						So(err, ShouldBeNil)

						So(task, ShouldNotBeNil)
						So(task, ShouldEqual, mockedTaskHandle)

						Convey("Location of the returned task shall be 127.0.0.1", func() {
							addr := task.Address()
							So(addr, ShouldEqual, expectedHost)
							mockedTaskHandle.AssertExpectations(t)
						})
						mockedExecutor.AssertExpectations(t)
					})
					Convey("When test connection to memcached fails task handle shall be nil and error shall be return", func() {
						mockedTaskHandle.On("Stop").Return(nil)
						mockedTaskHandle.On("Clean").Return(nil)
						memcachedLauncher.tryConnect = connectTimeoutFailure
						task, err := memcachedLauncher.Launch()
						So(err, ShouldNotBeNil)
						So(task, ShouldBeNil)

						mockedExecutor.AssertExpectations(t)
					})
					Convey("When test connection to memcached fails and task.Stop fails task handle shall be nil and error shall be return", func() {
						mockedTaskHandle.On("Stop").Return(errors.New("Test error code for stop"))
						mockedTaskHandle.On("Clean").Return(nil)
						memcachedLauncher.tryConnect = connectTimeoutFailure
						task, err := memcachedLauncher.Launch()
						So(err, ShouldNotBeNil)
						So(task, ShouldBeNil)

						mockedExecutor.AssertExpectations(t)
					})
					Convey("When test connection to memcached fails, task.Stop succeeds and task.Clean fails task handle shall be nil and error shall be return", func() {
						mockedTaskHandle.On("Stop").Return(nil)
						mockedTaskHandle.On("Clean").Return(errors.New("Test error code for clean"))
						memcachedLauncher.tryConnect = connectTimeoutFailure
						task, err := memcachedLauncher.Launch()
						So(err, ShouldNotBeNil)
						So(task, ShouldBeNil)

						mockedExecutor.AssertExpectations(t)
					})

				})

			})

			Convey("While simulating error execution", func() {
				mockedExecutor.On("Execute", expectedCommand).Return(nil, errors.New("test")).Once()

				Convey("Build command should create proper command", func() {
					command := memcachedLauncher.buildCommand()
					So(command, ShouldEqual, expectedCommand)

					Convey("Arguments passed to Executor should be a proper command", func() {
						task, err := memcachedLauncher.Launch()
						So(err, ShouldNotBeNil)
						So(err.Error(), ShouldEqual, "test")

						So(task, ShouldBeNil)

						mockedExecutor.AssertExpectations(t)
					})
				})

			})

		})
	})
}