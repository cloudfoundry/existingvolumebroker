package utils

import (
	"code.cloudfoundry.org/goshims/osshim/os_fake"
	"code.cloudfoundry.org/lager/lagertest"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("utils", func() {
	Context("#IsThereAProxy", func() {

		var proxy bool
		var fakeOs *os_fake.FakeOs
		var testLogger *lagertest.TestLogger

		BeforeEach(func() {
			fakeOs = &os_fake.FakeOs{}
			testLogger = lagertest.NewTestLogger("testlogger")
		})

		JustBeforeEach(func() {
			proxy = IsThereAProxy(fakeOs, testLogger)
		})

		Context("when proxy environment variables exist", func() {
			BeforeEach(func() {
				fakeOs.LookupEnvReturns("someproxy", true)
			})
			It("should return true", func() {
				Expect(fakeOs.LookupEnvArgsForCall(0)).To(Equal("https_proxy"))
				Expect(proxy).To(Equal(true))
				Expect(testLogger.Buffer()).To(gbytes.Say("someproxy"))
			})
		})

		Context("when proxy environment variables exist but with no value", func() {
			BeforeEach(func() {
				fakeOs.LookupEnvReturns("", true)
			})

			It("should return false", func() {
				Expect(fakeOs.LookupEnvArgsForCall(0)).To(Equal("https_proxy"))
				Expect(proxy).To(Equal(false))
			})
		})

		Context("when proxy environment variables does not exist", func() {
			BeforeEach(func() {
				fakeOs.LookupEnvReturns("", false)
			})

			It("should return true", func() {
				Expect(fakeOs.LookupEnvArgsForCall(0)).To(Equal("https_proxy"))
				Expect(proxy).To(Equal(false))
			})
		})
	})

})
