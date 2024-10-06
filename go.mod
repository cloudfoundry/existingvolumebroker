module code.cloudfoundry.org/existingvolumebroker

require (
	code.cloudfoundry.org/clock v1.14.0
	code.cloudfoundry.org/goshims v0.41.0
	code.cloudfoundry.org/lager/v3 v3.8.0
	code.cloudfoundry.org/service-broker-store v0.90.0
	code.cloudfoundry.org/volume-mount-options v0.97.0
	github.com/google/gofuzz v1.2.0
	github.com/maxbrunsfeld/counterfeiter/v6 v6.9.0
	github.com/onsi/ginkgo/v2 v2.20.2
	github.com/onsi/gomega v1.34.2
	github.com/pivotal-cf/brokerapi/v11 v11.0.9
	github.com/tedsuo/ifrit v0.0.0-20230516164442-7862c310ad26
)

require (
	code.cloudfoundry.org/credhub-cli v0.0.0-20240930130634-18c8d3fad4fd // indirect
	github.com/cloudfoundry/go-socks5 v0.0.0-20240831012420-2590b55236ee // indirect
	github.com/cloudfoundry/socks5-proxy v0.2.126 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/pprof v0.0.0-20241001023024-f4c0cfd0cf1d // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/openzipkin/zipkin-go v0.4.3 // indirect
	golang.org/x/crypto v0.28.0 // indirect
	golang.org/x/mod v0.21.0 // indirect
	golang.org/x/net v0.30.0 // indirect
	golang.org/x/sync v0.8.0 // indirect
	golang.org/x/sys v0.26.0 // indirect
	golang.org/x/text v0.19.0 // indirect
	golang.org/x/tools v0.26.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

go 1.23

toolchain go1.23.2
