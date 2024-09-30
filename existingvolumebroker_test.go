package existingvolumebroker_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"code.cloudfoundry.org/existingvolumebroker"
	"code.cloudfoundry.org/existingvolumebroker/fakes"
	"code.cloudfoundry.org/goshims/osshim/os_fake"
	"code.cloudfoundry.org/lager/v3/lagertest"
	"code.cloudfoundry.org/service-broker-store/brokerstore"
	"code.cloudfoundry.org/service-broker-store/brokerstore/brokerstorefakes"
	vmo "code.cloudfoundry.org/volume-mount-options"
	volumemountoptionsfakes "code.cloudfoundry.org/volume-mount-options/volume-mount-optionsfakes"
	fuzz "github.com/google/gofuzz"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/pivotal-cf/brokerapi/v11/domain"
	"github.com/pivotal-cf/brokerapi/v11/domain/apiresponses"
)

//counterfeiter:generate -o ./fakes/fake_user_opts_validation.go code.cloudfoundry.org/volume-mount-options.UserOptsValidation

var _ = Describe("Broker", func() {
	var (
		broker       domain.ServiceBroker
		fakeOs       *os_fake.FakeOs
		logger       *lagertest.TestLogger
		ctx          context.Context
		fakeStore    *brokerstorefakes.FakeStore
		fakeServices *fakes.FakeServices
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-broker")
		ctx = context.TODO()
		fakeOs = &os_fake.FakeOs{}
		fakeStore = &brokerstorefakes.FakeStore{}
		fakeServices = &fakes.FakeServices{}
	})

	Context("when the broker type is NFS", func() {
		var (
			err                error
			configMask         vmo.MountOptsMask
			userValidationFunc *volumemountoptionsfakes.FakeUserOptsValidation
		)

		BeforeEach(func() {
			userValidationFunc = &volumemountoptionsfakes.FakeUserOptsValidation{}
			fakeServices.ListReturns([]domain.Service{
				{
					ID:            "nfs-service-id",
					Name:          "nfs",
					Description:   "Existing NFSv3 volumes",
					Bindable:      true,
					PlanUpdatable: false,
					Tags:          []string{"nfs"},
					Requires:      []domain.RequiredPermission{"volume_mount"},
					Plans: []domain.ServicePlan{
						{
							Name:        "Existing",
							ID:          "Existing",
							Description: "A preexisting filesystem",
						},
					},
				},
				{
					ID:            "nfs-experimental-service-id",
					Name:          "nfs-experimental",
					Description:   "Experimental support for NFSv3 and v4",
					Bindable:      true,
					PlanUpdatable: false,
					Tags:          []string{"nfs", "experimental"},
					Requires:      []domain.RequiredPermission{"volume_mount"},

					Plans: []domain.ServicePlan{
						{
							Name:        "Existing",
							ID:          "Existing",
							Description: "A preexisting filesystem",
						},
					},
				},
			})
			configMask, err = vmo.NewMountOptsMask(
				[]string{
					"gid",
					"mount",
					"password",
					"source",
					"uid",
					"username",
					"version",
					"ro",
					"readonly",
				},
				map[string]interface{}{},
				map[string]string{
					"readonly": "ro",
					"share":    "source",
				},
				[]string{},
				[]string{"source"},
				userValidationFunc,
			)
			Expect(err).NotTo(HaveOccurred())

			broker = existingvolumebroker.New(
				existingvolumebroker.BrokerTypeNFS,
				logger,
				fakeServices,
				fakeOs,
				nil,
				fakeStore,
				configMask,
			)
		})

		Context(".Services", func() {
			It("returns the service catalog as appropriate", func() {
				results, err := broker.Services(ctx)
				Expect(err).NotTo(HaveOccurred())

				Expect(results).To(HaveLen(2))

				result := results[0]
				Expect(result.ID).To(Equal("nfs-service-id"))
				Expect(result.Name).To(Equal("nfs"))
				Expect(result.Description).To(Equal("Existing NFSv3 volumes"))
				Expect(result.Bindable).To(Equal(true))
				Expect(result.PlanUpdatable).To(Equal(false))
				Expect(result.Tags).To(ConsistOf([]string{"nfs"}))
				Expect(result.Requires).To(ContainElement(domain.RequiredPermission("volume_mount")))

				Expect(result.Plans[0].Name).To(Equal("Existing"))
				Expect(result.Plans[0].ID).To(Equal("Existing"))
				Expect(result.Plans[0].Description).To(Equal("A preexisting filesystem"))

				result = results[1]
				Expect(result.ID).To(Equal("nfs-experimental-service-id"))
				Expect(result.Name).To(Equal("nfs-experimental"))
				Expect(result.Tags).To(ConsistOf([]string{"nfs", "experimental"}))
				Expect(result.Requires).To(ContainElement(domain.RequiredPermission("volume_mount")))
			})
		})

		Context(".Provision", func() {
			var (
				instanceID       string
				provisionDetails domain.ProvisionDetails
				asyncAllowed     bool

				spec domain.ProvisionedServiceSpec
				err  error
			)

			BeforeEach(func() {
				instanceID = "some-instance-id"

				configuration := map[string]interface{}{"share": "server/some-share"}
				buf := &bytes.Buffer{}

				err = json.NewEncoder(buf).Encode(configuration)
				Expect(err).NotTo(HaveOccurred())

				provisionDetails = domain.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				asyncAllowed = false
				fakeStore.RetrieveInstanceDetailsReturns(brokerstore.ServiceInstance{}, errors.New("not found"))
			})

			JustBeforeEach(func() {
				spec, err = broker.Provision(ctx, instanceID, provisionDetails, asyncAllowed)
			})

			It("should not error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("should provision the service instance synchronously", func() {
				Expect(spec.IsAsync).To(Equal(false))
			})

			It("should write state", func() {
				Expect(fakeStore.SaveCallCount()).Should(BeNumerically(">", 0))
			})

			Context("when create service json contains uid and gid", func() {
				BeforeEach(func() {
					configuration := map[string]interface{}{"share": "server/some-share", "uid": "1", "gid": 2}
					buf := &bytes.Buffer{}

					err = json.NewEncoder(buf).Encode(configuration)
					Expect(err).NotTo(HaveOccurred())

					provisionDetails = domain.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				})

				It("should write uid and gid into state", func() {
					count := fakeStore.CreateInstanceDetailsCallCount()
					Expect(count).To(BeNumerically(">", 0))

					_, details := fakeStore.CreateInstanceDetailsArgsForCall(count - 1)

					fp := details.ServiceFingerPrint.(map[string]interface{})
					Expect(fp).NotTo(BeNil())
					Expect(fp).To(HaveKeyWithValue("uid", "1"))
					Expect(fp).To(HaveKeyWithValue("gid", float64(2)))
				})
			})

			Context("create-service was given invalid JSON", func() {
				BeforeEach(func() {
					badJson := []byte("{this is not json")
					provisionDetails = domain.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(badJson)}
				})

				It("errors", func() {
					Expect(err).To(Equal(apiresponses.ErrRawParamsInvalid))
				})

			})

			Context("create-service was given valid JSON but no 'share' key", func() {
				BeforeEach(func() {
					configuration := map[string]interface{}{"unknown key": "server:/some-share"}
					buf := &bytes.Buffer{}

					err = json.NewEncoder(buf).Encode(configuration)
					Expect(err).NotTo(HaveOccurred())

					provisionDetails = domain.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				})

				It("errors", func() {
					Expect(err).To(Equal(errors.New("config requires a \"share\" key")))
				})
			})

			Context("create-service was given a server share with colon after server", func() {
				BeforeEach(func() {
					configuration := map[string]interface{}{"share": "server:/some-share"}
					buf := &bytes.Buffer{}
					_ = json.NewEncoder(buf).Encode(configuration)
					provisionDetails = domain.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				})

				It("errors", func() {
					Expect(err).To(Equal(errors.New("syntax error for share: no colon allowed after server")))
				})
			})

			Context("create-service was given a server share with colon after nfs directory", func() {
				BeforeEach(func() {
					configuration := map[string]interface{}{"share": "server/some-share:dir/"}
					buf := &bytes.Buffer{}

					err = json.NewEncoder(buf).Encode(configuration)
					Expect(err).NotTo(HaveOccurred())

					provisionDetails = domain.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				})

				It("should not error", func() {
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when the service instance already exists with the same details", func() {
				BeforeEach(func() {
					fakeStore.IsInstanceConflictReturns(false)
				})

				It("should not error", func() {
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when the service instance already exists with different details", func() {
				BeforeEach(func() {
					fakeStore.IsInstanceConflictReturns(true)
				})

				It("should error", func() {
					Expect(err).To(Equal(apiresponses.ErrInstanceAlreadyExists))
				})
			})

			Context("when the service instance creation fails", func() {
				BeforeEach(func() {
					fakeStore.CreateInstanceDetailsReturns(errors.New("badness"))
				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})
			})

			Context("when the save fails", func() {
				BeforeEach(func() {
					fakeStore.SaveReturns(errors.New("badness"))
				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})
			})
		})

		Context(".Deprovision", func() {
			var (
				instanceID   string
				asyncAllowed bool
				err          error
			)

			BeforeEach(func() {
				instanceID = "some-instance-id"
				asyncAllowed = true
			})

			JustBeforeEach(func() {
				_, err = broker.Deprovision(ctx, instanceID, domain.DeprovisionDetails{}, asyncAllowed)
			})

			Context("when the instance does not exist", func() {
				BeforeEach(func() {
					instanceID = "does-not-exist"
					fakeStore.RetrieveInstanceDetailsReturns(brokerstore.ServiceInstance{}, apiresponses.ErrInstanceDoesNotExist)
				})

				It("should fail", func() {
					Expect(err).To(Equal(apiresponses.ErrInstanceDoesNotExist))
				})
			})

			Context("given an existing instance", func() {
				var (
					previousSaveCallCount int
				)

				BeforeEach(func() {
					instanceID = "some-instance-id"

					configuration := map[string]interface{}{"share": "server:/some-share"}
					buf := &bytes.Buffer{}

					err = json.NewEncoder(buf).Encode(configuration)
					Expect(err).NotTo(HaveOccurred())

					asyncAllowed = false
					fakeStore.RetrieveInstanceDetailsReturns(brokerstore.ServiceInstance{ServiceID: instanceID}, nil)
					previousSaveCallCount = fakeStore.SaveCallCount()
				})

				It("should succeed", func() {
					Expect(err).NotTo(HaveOccurred())
				})

				It("save state", func() {
					Expect(fakeStore.SaveCallCount()).To(Equal(previousSaveCallCount + 1))
				})

				Context("when deletion of the instance fails", func() {
					BeforeEach(func() {
						fakeStore.DeleteInstanceDetailsReturns(errors.New("badness"))
					})

					It("should error", func() {
						Expect(err).To(HaveOccurred())
					})
				})
			})

			Context("when the save fails", func() {
				BeforeEach(func() {
					fakeStore.SaveReturns(errors.New("badness"))
				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})
			})
		})

		Context(".LastOperation", func() {
			It("errors", func() {
				_, err := broker.LastOperation(ctx, "non-existant", domain.PollDetails{OperationData: "provision"})
				Expect(err).To(HaveOccurred())
			})
		})

		Context(".Bind", func() {
			var (
				instanceID, serviceID string
				bindDetails           domain.BindDetails
				bindParameters        map[string]interface{}

				uid, gid, username, password string
				fuzzer                       = fuzz.New()
			)

			BeforeEach(func() {
				fuzzer.Fuzz(&instanceID)
				fuzzer.Fuzz(&serviceID)
				fuzzer.Fuzz(&uid)
				fuzzer.Fuzz(&gid)
				fuzzer.Fuzz(&username)
				fuzzer.Fuzz(&password)

				serviceInstance := brokerstore.ServiceInstance{
					ServiceID: serviceID,
					ServiceFingerPrint: map[string]interface{}{
						existingvolumebroker.SHARE_KEY: "server/some-share",
					},
				}

				fakeStore.RetrieveInstanceDetailsReturns(serviceInstance, nil)
				fakeStore.RetrieveBindingDetailsReturns(domain.BindDetails{}, errors.New("yar"))

				bindParameters = map[string]interface{}{
					"username": username,
					"password": password,
					"uid":      uid,
					"gid":      gid,
				}

				bindMessage, err := json.Marshal(bindParameters)
				Expect(err).NotTo(HaveOccurred())

				bindDetails = domain.BindDetails{
					AppGUID:       "guid",
					RawParameters: bindMessage,
				}
			})

			for i := 0; i < 1000; i++ {
				It(fmt.Sprintf("passes `share` from create-service into the mount config on the bind response. Attempt :%v", i), func() {
					binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())

					mc := binding.VolumeMounts[0].Device.MountConfig

					// for backwards compatibility the nfs flavor has to issue source strings
					// with nfs:// prefix (otherwise the mapfsmounter wont construct the correct
					// mount string
					// see (https://github.com/cloudfoundry/nfsv3driver/blob/ac1e1d26fec9a8551cacfabafa6e035f233c83e0/mapfs_mounter.go#L121)
					v, ok := mc["source"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal("nfs://server/some-share"))

					v, ok = mc["username"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal(username))

					v, ok = mc["password"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal(password))

					v, ok = mc["uid"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal(uid))

					v, ok = mc["gid"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal(gid))
				})

				Context(fmt.Sprintf("when binddetails contains key/values that are not allowed. Attempt: %v", i), func() {
					BeforeEach(func() {
						var fuzzyParams map[string]string
						fuzzer = fuzzer.NumElements(5, 100).NilChance(0)
						fuzzer.Fuzz(&fuzzyParams)

						bindMessage, err := json.Marshal(fuzzyParams)
						Expect(err).NotTo(HaveOccurred())

						bindDetails = domain.BindDetails{
							AppGUID:       "guid",
							RawParameters: bindMessage,
						}
					})

					It(fmt.Sprintf("errors with a meaningful error message Attempt: %v", i), func() {
						_, err = broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)

						Expect(err).To(BeAssignableToTypeOf(&apiresponses.FailureResponse{}))
						Expect(err.(*apiresponses.FailureResponse).ValidatedStatusCode(nil)).To(Equal(400))
						Expect(err.(*apiresponses.FailureResponse).LoggerAction()).To(Equal("invalid-params"))

						Expect(err).To(MatchError(ContainSubstring("Not allowed options")))
					})
				})
			}

			It("includes empty credentials to prevent CAPI crash", func() {
				binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.Credentials).NotTo(BeNil())
			})

			It("uses the instance ID in the default container path", func() {
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].ContainerDir).To(Equal("/var/vcap/data/some-instance-id"))
			})

			It("passes the container path through", func() {
				var err error

				bindParameters["mount"] = "/var/vcap/otherdir/something"

				bindDetails.RawParameters, err = json.Marshal(bindParameters)
				Expect(err).NotTo(HaveOccurred())

				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].ContainerDir).To(Equal("/var/vcap/otherdir/something"))
			})

			It("uses rw as its default mode", func() {
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].Mode).To(Equal("rw"))
			})

			It("errors if mode is not a boolean", func() {
				var err error

				bindParameters["readonly"] = ""
				bindDetails.RawParameters, err = json.Marshal(bindParameters)
				Expect(err).NotTo(HaveOccurred())

				_, err = broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				Expect(err).To(MatchError(`invalid ro parameter value: ""`))
			})

			It("should write state", func() {
				previousSaveCallCount := fakeStore.SaveCallCount()

				_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeStore.SaveCallCount()).To(Equal(previousSaveCallCount + 1))
			})

			It("fills in the driver name", func() {
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].Driver).To(Equal("nfsv3driver"))
			})

			It("fills in the volume ID", func() {
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].Device.VolumeId).To(ContainSubstring("some-instance-id"))
			})

			It("errors when the service instance does not exist", func() {
				fakeStore.RetrieveInstanceDetailsReturns(brokerstore.ServiceInstance{}, errors.New("Awesome!"))

				_, err := broker.Bind(ctx, "nonexistent-instance-id", "binding-id", domain.BindDetails{AppGUID: "guid"}, false)
				Expect(err).To(Equal(apiresponses.ErrInstanceDoesNotExist))
			})

			It("errors when the app guid is not provided", func() {
				_, err := broker.Bind(ctx, "some-instance-id", "binding-id", domain.BindDetails{}, false)
				Expect(err).To(Equal(apiresponses.ErrAppGuidNotProvided))
			})

			Context("given readonly is specified", func() {
				BeforeEach(func() {
					bindParameters["readonly"] = true
					bindDetails.RawParameters, err = json.Marshal(bindParameters)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should set the mode to RO and the mount config readonly flag to true", func() {
					// SMB Broker/Driver run with this configuration of config mask (readonly canonicalized to ro)

					var binding domain.Binding

					binding, err = broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())
					Expect(binding.VolumeMounts[0].Mode).To(Equal("r"))
					Expect(binding.VolumeMounts[0].Device.MountConfig["ro"]).To(Equal("true"))
				})
				Context("given that the config mask does not specify a key permutation for readonly", func() {
					// NFS Broker/Driver run with this configuration of config mask (readonly left alone)

					BeforeEach(func() {
						delete(configMask.KeyPerms, "readonly")
					})
					It("should still set the mode to RO", func() {
						var binding domain.Binding

						binding, err = broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
						Expect(err).NotTo(HaveOccurred())
						Expect(binding.VolumeMounts[0].Mode).To(Equal("r"))
						Expect(binding.VolumeMounts[0].Device.MountConfig["readonly"]).To(Equal("true"))
					})
				})
			})

			Context("when the service instance contains uid and gid", func() {
				BeforeEach(func() {
					serviceInstance := brokerstore.ServiceInstance{
						ServiceID: serviceID,
						ServiceFingerPrint: map[string]interface{}{
							existingvolumebroker.SHARE_KEY: "server:/some-share",
							"uid":                          "1",
							"gid":                          2,
						},
					}

					fakeStore.RetrieveInstanceDetailsReturns(serviceInstance, nil)
				})

				It("should favor the values in the bind configuration", func() {
					binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())

					mc := binding.VolumeMounts[0].Device.MountConfig

					v, ok := mc["uid"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal(uid))

					v, ok = mc["gid"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal(gid))
				})

				Context("when the bind operation doesn't pass configuration", func() {
					BeforeEach(func() {
						bindDetails = domain.BindDetails{
							AppGUID:       "guid",
							RawParameters: []byte(""),
						}
					})

					It("should use uid and gid from the service instance configuration", func() {
						binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
						Expect(err).NotTo(HaveOccurred())

						mc := binding.VolumeMounts[0].Device.MountConfig

						v, ok := mc["uid"].(string)
						Expect(ok).To(BeTrue())
						Expect(v).To(Equal("1"))

						v, ok = mc["gid"].(string)
						Expect(ok).To(BeTrue())
						Expect(v).To(Equal("2"))
					})
				})
			})

			Context("when the service instance contains username and password", func() {
				BeforeEach(func() {
					serviceInstance := brokerstore.ServiceInstance{
						ServiceID: serviceID,
						ServiceFingerPrint: map[string]interface{}{
							existingvolumebroker.SHARE_KEY: "server:/some-share",
							"username":                     "some-instance-username",
							"password":                     "some-instance-password",
						},
					}

					fakeStore.RetrieveInstanceDetailsReturns(serviceInstance, nil)

					bindDetails = domain.BindDetails{
						AppGUID:       "guid",
						RawParameters: []byte(`{"username":"some-bind-username","password":"some-bind-password"}`),
					}
				})

				It("should favor the values in the bind configuration", func() {
					binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())

					mc := binding.VolumeMounts[0].Device.MountConfig

					v, ok := mc["username"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal("some-bind-username"))

					v, ok = mc["password"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal("some-bind-password"))
				})

				Context("when the bind operation doesn't pass configuration", func() {
					BeforeEach(func() {
						bindDetails = domain.BindDetails{
							AppGUID:       "guid",
							RawParameters: []byte(""),
						}
					})

					It("should use uid and gid from the service instance configuration", func() {
						binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
						Expect(err).NotTo(HaveOccurred())
						mc := binding.VolumeMounts[0].Device.MountConfig

						v, ok := mc["username"].(string)
						Expect(ok).To(BeTrue())
						Expect(v).To(Equal("some-instance-username"))

						v, ok = mc["password"].(string)
						Expect(ok).To(BeTrue())
						Expect(v).To(Equal("some-instance-password"))
					})
				})
			})

			Context("when the service instance contains a legacy service fingerprint", func() {
				BeforeEach(func() {
					serviceInstance := brokerstore.ServiceInstance{
						ServiceID:          serviceID,
						ServiceFingerPrint: "server:/some-share",
					}

					fakeStore.RetrieveInstanceDetailsReturns(serviceInstance, nil)

					bindDetails = domain.BindDetails{
						AppGUID:       "guid",
						RawParameters: []byte(`{"uid":"1000","gid":"1000"}`),
					}
				})

				It("should not error", func() {
					_, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when the bind operation doesn't pass configuration", func() {
				BeforeEach(func() {
					bindDetails = domain.BindDetails{
						AppGUID:       "guid",
						RawParameters: []byte(""),
					}
				})

				It("should succeed", func() {
					_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when the bind operation passes empty configuration", func() {
				BeforeEach(func() {
					bindDetails = domain.BindDetails{
						AppGUID:       "guid",
						RawParameters: []byte("{}"),
					}
				})

				It("should succeed", func() {
					_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when using nfs version", func() {
				BeforeEach(func() {
					serviceInstance := brokerstore.ServiceInstance{
						ServiceID: "nfs-experimental-service-id",
						ServiceFingerPrint: map[string]interface{}{
							existingvolumebroker.SHARE_KEY:   "server:/some-share",
							existingvolumebroker.VERSION_KEY: "4.1",
						},
					}

					fakeStore.RetrieveInstanceDetailsReturns(serviceInstance, nil)
				})

				It("includes version in the service binding mount config", func() {
					binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())

					mc := binding.VolumeMounts[0].Device.MountConfig
					version, ok := mc["version"]

					Expect(ok).To(BeTrue())
					Expect(version).To(Equal("4.1"))
				})
			})

			Context("when the binding already exists", func() {
				It("doesn't error when binding the same details", func() {
					fakeStore.IsBindingConflictReturns(false)

					_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())
				})

				It("errors when binding different details", func() {
					fakeStore.IsBindingConflictReturns(true)

					_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
					Expect(err).To(Equal(apiresponses.ErrBindingAlreadyExists))
				})
			})

			Context("given another binding with the same share", func() {
				var (
					err       error
					bindSpec1 domain.Binding
				)

				BeforeEach(func() {
					bindSpec1, err = broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())
				})

				Context("given different options", func() {
					var (
						bindSpec2 domain.Binding
					)

					BeforeEach(func() {
						var err error
						bindParameters["uid"] = "3000"
						bindParameters["gid"] = "3000"

						bindDetails.RawParameters, err = json.Marshal(bindParameters)
						Expect(err).NotTo(HaveOccurred())

						bindSpec2, err = broker.Bind(ctx, "some-instance-id", "binding-id-2", bindDetails, false)
						Expect(err).NotTo(HaveOccurred())
					})

					It("should issue a volume mount with a different volume ID", func() {
						Expect(bindSpec1.VolumeMounts[0].Device.VolumeId).NotTo(Equal(bindSpec2.VolumeMounts[0].Device.VolumeId))
					})
				})
			})

			Context("when the binding cannot be stored", func() {
				var (
					err error
				)

				BeforeEach(func() {
					fakeStore.CreateBindingDetailsReturns(errors.New("badness"))
					_, err = broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)

				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})
			})

			Context("when the save fails", func() {
				var (
					err error
				)

				BeforeEach(func() {
					fakeStore.SaveReturns(errors.New("badness"))
					_, err = broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})
			})

			Context("given allowed and default parameters are empty", func() {
				BeforeEach(func() {
					configMask, err := vmo.NewMountOptsMask(
						[]string{},
						map[string]interface{}{},
						map[string]string{
							"readonly": "ro",
							"share":    "source",
						},
						[]string{},
						[]string{"source"},
					)
					Expect(err).NotTo(HaveOccurred())

					broker = existingvolumebroker.New(
						existingvolumebroker.BrokerTypeNFS,
						logger,
						fakeServices,
						fakeOs,
						nil,
						fakeStore,
						configMask,
					)
				})
			})

			Context("given allowed and default parameters are empty, except for mount default with sloppy_mount=true is supplied ", func() {
				BeforeEach(func() {
					configMask, err := vmo.NewMountOptsMask(
						[]string{},
						map[string]interface{}{
							"sloppy_mount": "true",
						},
						map[string]string{
							"readonly": "ro",
							"share":    "source",
						},
						[]string{},
						[]string{},
					)
					Expect(err).NotTo(HaveOccurred())

					broker = existingvolumebroker.New(
						existingvolumebroker.BrokerTypeNFS,
						logger,
						fakeServices,
						fakeOs,
						nil,
						fakeStore,
						configMask,
					)
				})

			})

			Context("given default parameters are empty, allowed parameters contain allow_root", func() {
				BeforeEach(func() {
					configMask, err := vmo.NewMountOptsMask(
						[]string{
							"allow_root",
							"source",
						},
						map[string]interface{}{},
						map[string]string{
							"readonly": "ro",
							"share":    "source",
						},
						[]string{},
						[]string{"source"},
					)
					Expect(err).NotTo(HaveOccurred())

					broker = existingvolumebroker.New(
						existingvolumebroker.BrokerTypeNFS,
						logger,
						fakeServices,
						fakeOs,
						nil,
						fakeStore,
						configMask,
					)
				})

				Context("given allow_root=true is supplied", func() {
					BeforeEach(func() {
						bindParameters := map[string]interface{}{
							"allow_root": true,
						}

						bindMessage, err := json.Marshal(bindParameters)
						Expect(err).NotTo(HaveOccurred())

						bindDetails = domain.BindDetails{AppGUID: "guid", RawParameters: bindMessage}
					})

					It("passes allow_root=true option through", func() {
						binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
						Expect(err).NotTo(HaveOccurred())

						mc := binding.VolumeMounts[0].Device.MountConfig

						ar, ok := mc["allow_root"].(string)
						Expect(ok).To(BeTrue())
						Expect(ar).To(Equal("true"))
					})
				})
			})
		})

		Context(".Unbind", func() {
			var (
				bindDetails domain.BindDetails
			)

			BeforeEach(func() {
				bindParameters := map[string]interface{}{
					"uid": "1000",
					"gid": "1000",
				}

				bindMessage, err := json.Marshal(bindParameters)
				Expect(err).NotTo(HaveOccurred())
				bindDetails = domain.BindDetails{AppGUID: "guid", RawParameters: bindMessage}

				fakeStore.RetrieveBindingDetailsReturns(bindDetails, nil)
			})

			It("unbinds a bound service instance from an app", func() {
				_, err := broker.Unbind(ctx, "some-instance-id", "binding-id", domain.UnbindDetails{}, false)
				Expect(err).NotTo(HaveOccurred())
			})

			It("fails when trying to unbind a instance that has not been provisioned", func() {
				fakeStore.RetrieveInstanceDetailsReturns(brokerstore.ServiceInstance{}, errors.New("Shazaam!"))

				_, err := broker.Unbind(ctx, "some-other-instance-id", "binding-id", domain.UnbindDetails{}, false)
				Expect(err).To(Equal(apiresponses.ErrInstanceDoesNotExist))
			})

			It("fails when trying to unbind a binding that has not been bound", func() {
				fakeStore.RetrieveBindingDetailsReturns(domain.BindDetails{}, errors.New("Hooray!"))

				_, err := broker.Unbind(ctx, "some-instance-id", "some-other-binding-id", domain.UnbindDetails{}, false)
				Expect(err).To(Equal(apiresponses.ErrBindingDoesNotExist))
			})

			It("should write state", func() {
				previousCallCount := fakeStore.SaveCallCount()

				_, err := broker.Unbind(ctx, "some-instance-id", "binding-id", domain.UnbindDetails{}, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeStore.SaveCallCount()).To(Equal(previousCallCount + 1))
			})

			Context("when the save fails", func() {
				BeforeEach(func() {
					fakeStore.SaveReturns(errors.New("badness"))
				})

				It("should error", func() {
					_, err := broker.Unbind(ctx, "some-instance-id", "binding-id", domain.UnbindDetails{}, false)
					Expect(err).To(HaveOccurred())
				})
			})

			Context("when deletion of the binding details fails", func() {
				BeforeEach(func() {
					fakeStore.DeleteBindingDetailsReturns(errors.New("badness"))
				})

				It("should error", func() {
					_, err := broker.Unbind(ctx, "some-instance-id", "binding-id", domain.UnbindDetails{}, false)
					Expect(err).To(HaveOccurred())
				})
			})
		})

		Context(".Update", func() {
			It("should return a 422 status code", func() {
				_, err := broker.Update(ctx, "", domain.UpdateDetails{}, false)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(apiresponses.NewFailureResponse(errors.New(
					"this service does not support instance updates. Please delete your service instance and create a new one with updated configuration."),
					422,
					"")))
			})
		})
	})

	Context("when the broker type is SMB", func() {
		BeforeEach(func() {
			fakeServices.ListReturns([]domain.Service{
				{
					ID:            "smb-service-id",
					Name:          "smb",
					Description:   "Existing SMB shares",
					Bindable:      true,
					PlanUpdatable: false,
					Tags:          []string{"smb"},
					Requires:      []domain.RequiredPermission{"volume_mount"},
					Plans: []domain.ServicePlan{
						{
							Name:        "Existing",
							ID:          "Existing",
							Description: "A preexisting filesystem",
						},
					},
				},
			})

			configMask, err := vmo.NewMountOptsMask(
				[]string{
					"domain",
					"gid",
					"mount",
					"password",
					"ro",
					"source",
					"uid",
					"username",
				},
				map[string]interface{}{},
				map[string]string{
					"readonly": "ro",
					"share":    "source",
				},
				[]string{},
				[]string{"source"},
			)
			Expect(err).NotTo(HaveOccurred())

			broker = existingvolumebroker.New(
				existingvolumebroker.BrokerTypeSMB,
				logger,
				fakeServices,
				fakeOs,
				nil,
				fakeStore,
				configMask,
			)
		})

		Context(".Services", func() {
			It("returns the service catalog as appropriate", func() {
				results, err := broker.Services(ctx)
				Expect(err).NotTo(HaveOccurred())

				Expect(results).To(HaveLen(1))

				result := results[0]
				Expect(result.ID).To(Equal("smb-service-id"))
				Expect(result.Name).To(Equal("smb"))
				Expect(result.Description).To(Equal("Existing SMB shares"))
				Expect(result.Bindable).To(Equal(true))
				Expect(result.PlanUpdatable).To(Equal(false))
				Expect(result.Tags).To(ConsistOf([]string{"smb"}))
				Expect(result.Requires).To(ContainElement(domain.RequiredPermission("volume_mount")))

				Expect(result.Plans[0].Name).To(Equal("Existing"))
				Expect(result.Plans[0].ID).To(Equal("Existing"))
				Expect(result.Plans[0].Description).To(Equal("A preexisting filesystem"))
			})
		})

		Context(".Provision", func() {
			var (
				instanceID       string
				provisionDetails domain.ProvisionDetails
				asyncAllowed     bool

				spec domain.ProvisionedServiceSpec
				err  error
			)

			BeforeEach(func() {
				instanceID = "some-instance-id"

				configuration := map[string]interface{}{"share": "server/some-share"}
				buf := &bytes.Buffer{}

				err = json.NewEncoder(buf).Encode(configuration)
				Expect(err).NotTo(HaveOccurred())

				provisionDetails = domain.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				asyncAllowed = false
				fakeStore.RetrieveInstanceDetailsReturns(brokerstore.ServiceInstance{}, errors.New("not found"))
			})

			JustBeforeEach(func() {
				spec, err = broker.Provision(ctx, instanceID, provisionDetails, asyncAllowed)
			})

			It("should not error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("should provision the service instance synchronously", func() {
				Expect(spec.IsAsync).To(Equal(false))
			})

			It("should write state", func() {
				Expect(fakeStore.SaveCallCount()).Should(BeNumerically(">", 0))
			})

			Context("when create service json contains uid and gid", func() {
				BeforeEach(func() {
					configuration := map[string]interface{}{
						"share": "server/some-share",
						"uid":   "1",
						"gid":   2,
					}
					buf := &bytes.Buffer{}

					err = json.NewEncoder(buf).Encode(configuration)
					Expect(err).NotTo(HaveOccurred())

					provisionDetails = domain.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				})

				It("should write uid and gid into state", func() {
					count := fakeStore.CreateInstanceDetailsCallCount()
					Expect(count).To(BeNumerically(">", 0))

					_, details := fakeStore.CreateInstanceDetailsArgsForCall(count - 1)

					fp := details.ServiceFingerPrint.(map[string]interface{})
					Expect(fp).NotTo(BeNil())
					Expect(fp).To(HaveKeyWithValue("uid", "1"))
					Expect(fp).To(HaveKeyWithValue("gid", float64(2)))
				})
			})

			Context("create-service was given invalid JSON", func() {
				BeforeEach(func() {
					badJson := []byte("{this is not json")
					provisionDetails = domain.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(badJson)}
				})

				It("errors", func() {
					Expect(err).To(Equal(apiresponses.ErrRawParamsInvalid))
				})

			})

			Context("create-service was given valid JSON but no 'share' key", func() {
				BeforeEach(func() {
					configuration := map[string]interface{}{"unknown key": "server:/some-share"}
					buf := &bytes.Buffer{}

					err = json.NewEncoder(buf).Encode(configuration)
					Expect(err).NotTo(HaveOccurred())

					provisionDetails = domain.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				})

				It("errors", func() {
					Expect(err).To(Equal(errors.New("config requires a \"share\" key")))
				})
			})

			Context("create-service was given valid JSON with a 'source' key", func() {
				BeforeEach(func() {
					configuration := map[string]interface{}{"source": "server:/some-share", "share": "server:/some-share"}
					buf := &bytes.Buffer{}

					err = json.NewEncoder(buf).Encode(configuration)
					Expect(err).NotTo(HaveOccurred())

					provisionDetails = domain.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				})

				It("errors", func() {
					Expect(err).To(Equal(errors.New("create configuration contains the following invalid option: ['source']")))
				})
			})

			Context("create-service was given a server share with colon after nfs directory", func() {
				BeforeEach(func() {
					configuration := map[string]interface{}{"share": "server/some-share:dir/"}
					buf := &bytes.Buffer{}

					err = json.NewEncoder(buf).Encode(configuration)
					Expect(err).NotTo(HaveOccurred())

					provisionDetails = domain.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				})

				It("should not error", func() {
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when the service instance already exists with the same details", func() {
				BeforeEach(func() {
					fakeStore.IsInstanceConflictReturns(false)
				})

				It("should not error", func() {
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when the service instance already exists with different details", func() {
				BeforeEach(func() {
					fakeStore.IsInstanceConflictReturns(true)
				})

				It("should error", func() {
					Expect(err).To(Equal(apiresponses.ErrInstanceAlreadyExists))
				})
			})

			Context("when the service instance creation fails", func() {
				BeforeEach(func() {
					fakeStore.CreateInstanceDetailsReturns(errors.New("badness"))
				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})
			})

			Context("when the save fails", func() {
				BeforeEach(func() {
					fakeStore.SaveReturns(errors.New("badness"))
				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})
			})
		})

		Context(".Deprovision", func() {
			var (
				instanceID   string
				asyncAllowed bool
				err          error
			)

			BeforeEach(func() {
				instanceID = "some-instance-id"
				asyncAllowed = true
			})

			JustBeforeEach(func() {
				_, err = broker.Deprovision(ctx, instanceID, domain.DeprovisionDetails{}, asyncAllowed)
			})

			Context("when the instance does not exist", func() {
				BeforeEach(func() {
					instanceID = "does-not-exist"
					fakeStore.RetrieveInstanceDetailsReturns(brokerstore.ServiceInstance{}, apiresponses.ErrInstanceDoesNotExist)
				})

				It("should fail", func() {
					Expect(err).To(Equal(apiresponses.ErrInstanceDoesNotExist))
				})
			})

			Context("given an existing instance", func() {
				var (
					previousSaveCallCount int
				)

				BeforeEach(func() {
					instanceID = "some-instance-id"

					configuration := map[string]interface{}{"share": "server:/some-share"}
					buf := &bytes.Buffer{}

					err = json.NewEncoder(buf).Encode(configuration)
					Expect(err).NotTo(HaveOccurred())

					asyncAllowed = false
					fakeStore.RetrieveInstanceDetailsReturns(brokerstore.ServiceInstance{ServiceID: instanceID}, nil)
					previousSaveCallCount = fakeStore.SaveCallCount()
				})

				It("should succeed", func() {
					Expect(err).NotTo(HaveOccurred())
				})

				It("save state", func() {
					Expect(fakeStore.SaveCallCount()).To(Equal(previousSaveCallCount + 1))
				})

				Context("when deletion of the instance fails", func() {
					BeforeEach(func() {
						fakeStore.DeleteInstanceDetailsReturns(errors.New("badness"))
					})

					It("should error", func() {
						Expect(err).To(HaveOccurred())
					})
				})
			})

			Context("when the save fails", func() {
				BeforeEach(func() {
					fakeStore.SaveReturns(errors.New("badness"))
				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})
			})
		})

		Context(".LastOperation", func() {
			It("errors", func() {
				_, err := broker.LastOperation(ctx, "non-existant", domain.PollDetails{OperationData: "provision"})
				Expect(err).To(HaveOccurred())
			})
		})

		Context(".Bind", func() {
			var (
				instanceID, serviceID string
				bindDetails           domain.BindDetails
				bindParameters        map[string]interface{}

				username, password, ntDomain, uid, gid string
				fuzzer                                 = fuzz.New()
			)

			BeforeEach(func() {
				fuzzer.Fuzz(&instanceID)
				fuzzer.Fuzz(&serviceID)
				fuzzer.Fuzz(&uid)
				fuzzer.Fuzz(&gid)
				fuzzer.Fuzz(&username)
				fuzzer.Fuzz(&password)
				fuzzer.Fuzz(&ntDomain)

				fakeStore.RetrieveInstanceDetailsStub = func(instanceID string) (brokerstore.ServiceInstance, error) {
					return brokerstore.ServiceInstance{
						ServiceID: serviceID,
						ServiceFingerPrint: map[string]interface{}{
							existingvolumebroker.SHARE_KEY: "server:/some-share",
						},
					}, nil
				}
				fakeStore.RetrieveBindingDetailsReturns(domain.BindDetails{}, errors.New("yar"))

				bindParameters = map[string]interface{}{
					"username": username,
					"password": password,
					"domain":   ntDomain,
					"uid":      uid,
					"gid":      gid,
				}

				bindMessage, err := json.Marshal(bindParameters)
				Expect(err).NotTo(HaveOccurred())

				bindDetails = domain.BindDetails{
					AppGUID:       "guid",
					RawParameters: bindMessage,
				}
			})

			for i := 0; i < 1000; i++ {
				It(fmt.Sprintf("passes source, username, password, uid, gid and domain from create-service into mountConfig on the bind response. Attempt: %v", i), func() {
					binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())

					mc := binding.VolumeMounts[0].Device.MountConfig

					v, ok := mc["source"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal("server:/some-share"))

					v, ok = mc["username"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal(username))

					v, ok = mc["password"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal(password))

					v, ok = mc["domain"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal(ntDomain))

					v, ok = mc["uid"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal(uid))

					v, ok = mc["gid"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal(gid))
				})

				Context(fmt.Sprintf("when binddetails contains key/values that are not allowed. Attempt: %v", i), func() {
					BeforeEach(func() {
						var fuzzyParams map[string]string
						fuzzer = fuzzer.NumElements(5, 100).NilChance(0)
						fuzzer.Fuzz(&fuzzyParams)

						bindMessage, err := json.Marshal(fuzzyParams)
						Expect(err).NotTo(HaveOccurred())

						bindDetails = domain.BindDetails{
							AppGUID:       "guid",
							RawParameters: bindMessage,
						}
					})

					It(fmt.Sprintf("errors with a meaningful error message Attempt: %v", i), func() {
						_, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)

						Expect(err).To(MatchError(ContainSubstring("Not allowed options")))
					})
				})
			}

			It("includes empty credentials to prevent CAPI crash", func() {
				binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.Credentials).NotTo(BeNil())
			})

			It("uses the instance ID in the default container path", func() {
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].ContainerDir).To(Equal("/var/vcap/data/some-instance-id"))
			})

			It("passes the container path through", func() {
				var err error

				bindParameters["mount"] = "/var/vcap/otherdir/something"

				bindDetails.RawParameters, err = json.Marshal(bindParameters)
				Expect(err).NotTo(HaveOccurred())

				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].ContainerDir).To(Equal("/var/vcap/otherdir/something"))
			})

			It("uses rw as its default mode", func() {
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].Mode).To(Equal("rw"))
				Expect(binding.VolumeMounts[0].Device.MountConfig).NotTo(HaveKey("ro"))
			})

			It("sets mode to `r` when readonly is true", func() {
				var err error

				bindParameters["readonly"] = true
				bindDetails.RawParameters, err = json.Marshal(bindParameters)
				Expect(err).NotTo(HaveOccurred())

				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].Mode).To(Equal("r"))
				Expect(binding.VolumeMounts[0].Device.MountConfig).To(HaveKeyWithValue("ro", "true"))
			})

			It("should write state", func() {
				previousSaveCallCount := fakeStore.SaveCallCount()

				_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeStore.SaveCallCount()).To(Equal(previousSaveCallCount + 1))
			})

			It("errors if readonly is not true", func() {
				var err error

				bindParameters["readonly"] = ""
				bindDetails.RawParameters, err = json.Marshal(bindParameters)
				Expect(err).NotTo(HaveOccurred())

				_, err = broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				Expect(err).To(MatchError(`invalid ro parameter value: ""`))
			})

			It("fills in the driver name", func() {
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].Driver).To(Equal("smbdriver"))
			})

			It("fills in the volume ID", func() {
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].Device.VolumeId).To(ContainSubstring("some-instance-id"))
			})

			Context("when the service instance contains uid and gid", func() {
				BeforeEach(func() {
					fakeStore.RetrieveInstanceDetailsStub = func(instanceID string) (brokerstore.ServiceInstance, error) {
						return brokerstore.ServiceInstance{
							ServiceID: serviceID,
							ServiceFingerPrint: map[string]interface{}{
								existingvolumebroker.SHARE_KEY: "server:/some-share",
								"uid":                          "1",
								"gid":                          2,
							},
						}, nil
					}
				})

				It("should favor the values in the bind configuration", func() {
					binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())

					mc := binding.VolumeMounts[0].Device.MountConfig

					v, ok := mc["uid"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal(uid))

					v, ok = mc["gid"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal(gid))
				})

				Context("when the bind operation doesn't pass configuration", func() {
					BeforeEach(func() {
						bindDetails = domain.BindDetails{
							AppGUID:       "guid",
							RawParameters: []byte(""),
						}
					})

					It("should use uid and gid from the service instance configuration", func() {
						binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
						Expect(err).NotTo(HaveOccurred())

						mc := binding.VolumeMounts[0].Device.MountConfig

						v, ok := mc["uid"].(string)
						Expect(ok).To(BeTrue())
						Expect(v).To(Equal("1"))

						v, ok = mc["gid"].(string)
						Expect(ok).To(BeTrue())
						Expect(v).To(Equal("2"))
					})
				})
			})

			Context("when the service instance contains domain, username and password", func() {
				BeforeEach(func() {
					fakeStore.RetrieveInstanceDetailsStub = func(instanceID string) (brokerstore.ServiceInstance, error) {
						return brokerstore.ServiceInstance{
							ServiceID: serviceID,
							ServiceFingerPrint: map[string]interface{}{
								existingvolumebroker.SHARE_KEY: "server:/some-share",
								"domain":                       "some-instance-domain",
								"username":                     "some-instance-username",
								"password":                     "some-instance-password",
							},
						}, nil
					}

					bindDetails = domain.BindDetails{
						AppGUID:       "guid",
						RawParameters: []byte(`{"domain":"some-bind-domain","username":"some-bind-username","password":"some-bind-password"}`),
					}
				})

				It("should favor the values in the bind configuration", func() {
					binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())

					mc := binding.VolumeMounts[0].Device.MountConfig

					Expect(mc["domain"]).To(Equal("some-bind-domain"))
					Expect(mc["username"]).To(Equal("some-bind-username"))
					Expect(mc["password"]).To(Equal("some-bind-password"))
				})

				DescribeTable("when the bind configuration is set with a disallowed bind option", func(bindParamKeyName string) {
					serviceInstance := brokerstore.ServiceInstance{
						ServiceID:          serviceID,
						ServiceFingerPrint: map[string]interface{}{},
					}

					fakeStore.RetrieveInstanceDetailsReturns(serviceInstance, nil)

					bindParameters = map[string]interface{}{
						bindParamKeyName: "server:/some-other-share",
					}

					bindMessage, err := json.Marshal(bindParameters)
					Expect(err).NotTo(HaveOccurred())

					bindDetails = domain.BindDetails{
						AppGUID:       "guid",
						RawParameters: bindMessage,
					}

					_, err = broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError("bind configuration contains the following invalid option: ['" + bindParamKeyName + "']"))
					Expect(err).To(BeAssignableToTypeOf(&apiresponses.FailureResponse{}))
					Expect(err.(*apiresponses.FailureResponse).ValidatedStatusCode(nil)).To(Equal(400))
					Expect(logger.Buffer()).To(gbytes.Say("bind configuration contains the following invalid option: \\['" + bindParamKeyName + "'\\]"))

				},
					Entry("when the bind configuration overrides the source", "source"),
					Entry("when the bind configuration overrides the share", "share"),
				)

				Context("when the bind configuration is empty", func() {
					BeforeEach(func() {
						serviceInstance := brokerstore.ServiceInstance{
							ServiceID: serviceID,
							ServiceFingerPrint: map[string]interface{}{
								existingvolumebroker.SHARE_KEY: "server:/some-share",
								"domain":                       "some-instance-domain",
								"username":                     "some-instance-username",
								"password":                     "some-instance-password",
							},
						}

						fakeStore.RetrieveInstanceDetailsReturns(serviceInstance, nil)

						bindDetails = domain.BindDetails{
							AppGUID:       "guid",
							RawParameters: []byte{},
						}
					})

					It("should use the values in the service instance configuration", func() {
						binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
						Expect(err).NotTo(HaveOccurred())

						mc := binding.VolumeMounts[0].Device.MountConfig

						Expect(mc["domain"]).To(Equal("some-instance-domain"))
						Expect(mc["username"]).To(Equal("some-instance-username"))
						Expect(mc["password"]).To(Equal("some-instance-password"))
					})
				})
			})

			Context("when the bind operation doesn't pass configuration", func() {
				BeforeEach(func() {
					bindDetails = domain.BindDetails{
						AppGUID:       "guid",
						RawParameters: []byte(""),
					}
				})

				It("should succeed", func() {
					_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when the bind operation passes empty configuration", func() {
				BeforeEach(func() {
					bindDetails = domain.BindDetails{
						AppGUID:       "guid",
						RawParameters: []byte("{}"),
					}
				})

				It("should succeed", func() {
					_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when the bind configuration contains non-string values", func() {
				BeforeEach(func() {
					serviceInstance := brokerstore.ServiceInstance{
						ServiceID: serviceID,
						ServiceFingerPrint: map[string]interface{}{
							existingvolumebroker.SHARE_KEY: "server:/some-share",
							"username":                     "some-instance-username",
							"password":                     "some-instance-password",
						},
					}

					fakeStore.RetrieveInstanceDetailsReturns(serviceInstance, nil)

					bindDetails = domain.BindDetails{
						AppGUID:       "guid",
						RawParameters: []byte(`{"username":123,"password":false}`),
					}
				})

				It("should convert the bind configuration values to strings", func() {
					binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())

					mc := binding.VolumeMounts[0].Device.MountConfig

					Expect(mc["username"]).To(Equal("123"))
					Expect(mc["password"]).To(Equal("false"))
				})
			})

			Context("when the binding already exists", func() {
				It("doesn't error when binding the same details", func() {
					fakeStore.IsBindingConflictReturns(false)

					_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())
				})

				It("errors when binding different details", func() {
					fakeStore.IsBindingConflictReturns(true)

					_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
					Expect(err).To(Equal(apiresponses.ErrBindingAlreadyExists))
				})
			})

			Context("given another binding with the same share", func() {
				var (
					err       error
					bindSpec1 domain.Binding
				)

				BeforeEach(func() {
					bindSpec1, err = broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
					Expect(err).NotTo(HaveOccurred())
				})

				Context("given different options", func() {
					var (
						bindSpec2 domain.Binding
					)

					BeforeEach(func() {
						var err error
						bindParameters["username"] = "other-username"
						bindParameters["password"] = "other-password"

						bindDetails.RawParameters, err = json.Marshal(bindParameters)
						Expect(err).NotTo(HaveOccurred())

						bindSpec2, err = broker.Bind(ctx, "some-instance-id", "binding-id-2", bindDetails, false)
						Expect(err).NotTo(HaveOccurred())
					})

					It("should issue a volume mount with a different volume ID", func() {
						Expect(bindSpec1.VolumeMounts[0].Device.VolumeId).NotTo(Equal(bindSpec2.VolumeMounts[0].Device.VolumeId))
					})
				})
			})

			Context("when the binding cannot be stored", func() {
				var (
					err error
				)

				BeforeEach(func() {
					fakeStore.CreateBindingDetailsReturns(errors.New("badness"))
					_, err = broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)

				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})
			})

			Context("when the save fails", func() {
				var (
					err error
				)

				BeforeEach(func() {
					fakeStore.SaveReturns(errors.New("badness"))
					_, err = broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails, false)
				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})
			})

			It("errors when the service instance does not exist", func() {
				fakeStore.RetrieveInstanceDetailsStub = func(instanceID string) (brokerstore.ServiceInstance, error) {
					return brokerstore.ServiceInstance{}, errors.New("Awesome!")
				}

				_, err := broker.Bind(ctx, "nonexistent-instance-id", "binding-id", domain.BindDetails{AppGUID: "guid"}, false)
				Expect(err).To(Equal(apiresponses.ErrInstanceDoesNotExist))
			})

			It("errors when the app guid is not provided", func() {
				_, err := broker.Bind(ctx, "some-instance-id", "binding-id", domain.BindDetails{}, false)
				Expect(err).To(Equal(apiresponses.ErrAppGuidNotProvided))
			})

			Context("given allowed and default parameters are empty", func() {
				BeforeEach(func() {
					configMask, err := vmo.NewMountOptsMask(
						[]string{},
						map[string]interface{}{},
						map[string]string{
							"readonly": "ro",
							"share":    "source",
						},
						[]string{},
						[]string{"source"},
					)
					Expect(err).NotTo(HaveOccurred())

					broker = existingvolumebroker.New(
						existingvolumebroker.BrokerTypeSMB,
						logger,
						fakeServices,
						fakeOs,
						nil,
						fakeStore,
						configMask,
					)
				})

				Context("given allow_root=true is supplied", func() {
					BeforeEach(func() {
						bindParameters := map[string]interface{}{
							"allow_root": true,
						}

						bindMessage, err := json.Marshal(bindParameters)
						Expect(err).NotTo(HaveOccurred())
						bindDetails = domain.BindDetails{AppGUID: "guid", RawParameters: bindMessage}
					})

					It("should return with an error", func() {
						_, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
						Expect(err).To(HaveOccurred())
					})
				})
			})

			Context("given default parameters are empty, allowed parameters contain allow_root", func() {
				BeforeEach(func() {
					configMask, err := vmo.NewMountOptsMask(
						[]string{
							"allow_root",
							"source",
						},
						map[string]interface{}{},
						map[string]string{
							"readonly": "ro",
							"share":    "source",
						},
						[]string{},
						[]string{"source"},
					)
					Expect(err).NotTo(HaveOccurred())

					broker = existingvolumebroker.New(
						existingvolumebroker.BrokerTypeSMB,
						logger,
						fakeServices,
						fakeOs,
						nil,
						fakeStore,
						configMask,
					)
				})

				Context("given allow_root=true is supplied", func() {
					BeforeEach(func() {
						bindParameters := map[string]interface{}{
							"allow_root": true,
						}

						bindMessage, err := json.Marshal(bindParameters)
						Expect(err).NotTo(HaveOccurred())

						bindDetails = domain.BindDetails{AppGUID: "guid", RawParameters: bindMessage}
					})

					It("passes allow_root=true option through", func() {
						binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails, false)
						Expect(err).NotTo(HaveOccurred())

						mc := binding.VolumeMounts[0].Device.MountConfig

						ar, ok := mc["allow_root"].(string)
						Expect(ok).To(BeTrue())
						Expect(ar).To(Equal("true"))
					})
				})
			})
		})

		Context(".Unbind", func() {
			var (
				bindDetails domain.BindDetails
			)

			BeforeEach(func() {
				bindParameters := map[string]interface{}{
					"uid": "1000",
					"gid": "1000",
				}

				bindMessage, err := json.Marshal(bindParameters)
				Expect(err).NotTo(HaveOccurred())
				bindDetails = domain.BindDetails{AppGUID: "guid", RawParameters: bindMessage}

				fakeStore.RetrieveBindingDetailsReturns(bindDetails, nil)
			})

			It("unbinds a bound service instance from an app", func() {
				_, err := broker.Unbind(ctx, "some-instance-id", "binding-id", domain.UnbindDetails{}, false)
				Expect(err).NotTo(HaveOccurred())
			})

			It("fails when trying to unbind a instance that has not been provisioned", func() {
				fakeStore.RetrieveInstanceDetailsReturns(brokerstore.ServiceInstance{}, errors.New("Shazaam!"))

				_, err := broker.Unbind(ctx, "some-other-instance-id", "binding-id", domain.UnbindDetails{}, false)
				Expect(err).To(Equal(apiresponses.ErrInstanceDoesNotExist))
			})

			It("fails when trying to unbind a binding that has not been bound", func() {
				fakeStore.RetrieveBindingDetailsReturns(domain.BindDetails{}, errors.New("Hooray!"))

				_, err := broker.Unbind(ctx, "some-instance-id", "some-other-binding-id", domain.UnbindDetails{}, false)
				Expect(err).To(Equal(apiresponses.ErrBindingDoesNotExist))
			})

			It("should write state", func() {
				previousCallCount := fakeStore.SaveCallCount()

				_, err := broker.Unbind(ctx, "some-instance-id", "binding-id", domain.UnbindDetails{}, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeStore.SaveCallCount()).To(Equal(previousCallCount + 1))
			})

			Context("when the save fails", func() {
				BeforeEach(func() {
					fakeStore.SaveReturns(errors.New("badness"))
				})

				It("should error", func() {
					_, err := broker.Unbind(ctx, "some-instance-id", "binding-id", domain.UnbindDetails{}, false)
					Expect(err).To(HaveOccurred())
				})
			})

			Context("when deletion of the binding details fails", func() {
				BeforeEach(func() {
					fakeStore.DeleteBindingDetailsReturns(errors.New("badness"))
				})

				It("should error", func() {
					_, err := broker.Unbind(ctx, "some-instance-id", "binding-id", domain.UnbindDetails{}, false)
					Expect(err).To(HaveOccurred())
				})
			})
		})
	})
})
