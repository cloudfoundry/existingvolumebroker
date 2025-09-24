module code.cloudfoundry.org/existingvolumebroker

require (
	code.cloudfoundry.org/brokerapi/v13 v13.0.9
	code.cloudfoundry.org/clock v1.39.0
	code.cloudfoundry.org/goshims v0.69.0
	code.cloudfoundry.org/lager/v3 v3.38.0
	code.cloudfoundry.org/service-broker-store v0.120.0
	code.cloudfoundry.org/volume-mount-options v0.124.0
	github.com/google/gofuzz v1.2.0
	github.com/maxbrunsfeld/counterfeiter/v6 v6.12.0
	github.com/onsi/ginkgo/v2 v2.25.3
	github.com/onsi/gomega v1.38.2
	github.com/tedsuo/ifrit v0.0.0-20230516164442-7862c310ad26
)

require (
	code.cloudfoundry.org/credhub-cli v0.0.0-20250602130228-69976b334788 // indirect
	github.com/Masterminds/semver/v3 v3.4.0 // indirect
	github.com/cloudfoundry/go-socks5 v0.0.0-20250423223041-4ad5fea42851 // indirect
	github.com/cloudfoundry/socks5-proxy v0.2.153 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20250820193118-f64d9cf942d6 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/openzipkin/zipkin-go v0.4.3 // indirect
	github.com/pivotal-cf/brokerapi/v11 v11.0.16 // indirect
	go.uber.org/automaxprocs v1.6.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/mod v0.27.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.29.0 // indirect
	golang.org/x/tools v0.36.0 // indirect
)

go 1.24.0

toolchain go1.24.7
