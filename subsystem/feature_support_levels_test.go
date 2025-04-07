package subsystem

import (
	"context"
	"fmt"

	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift/assisted-service/client/installer"
	"github.com/openshift/assisted-service/internal/common"
	"github.com/openshift/assisted-service/internal/featuresupport"
	"github.com/openshift/assisted-service/models"
	"github.com/openshift/assisted-service/subsystem/utils_test"
	"github.com/thoas/go-funk"
)

var _ = Describe("Feature support levels API", func() {
	var ctx context.Context
	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("Support Level", func() {
		featureSupport := func(openshiftVersion string, platformType *string, cpuArchitecture *string) (*models.SupportLevels, error) {
			params := installer.GetSupportedFeaturesParams{
				OpenshiftVersion: openshiftVersion,
				PlatformType:     platformType,
				CPUArchitecture:  cpuArchitecture,
			}
			supportedFeaturesOK, err := utils_test.TestContext.User2BMClient.Installer.GetSupportedFeatures(ctx, &params)
			if err != nil {
				return nil, err
			}

			return &supportedFeaturesOK.Payload.Features, nil
		}

		registerNewCluster := func(version, cpuArchitecture string, ctrlPlaneCount int64, userManagedNetworking *bool) (*installer.V2RegisterClusterCreated, error) {
			cluster, errRegisterCluster := utils_test.TestContext.User2BMClient.Installer.V2RegisterCluster(ctx, &installer.V2RegisterClusterParams{
				NewClusterParams: &models.ClusterCreateParams{
					Name:                  swag.String("test-cluster"),
					OpenshiftVersion:      swag.String(version),
					PullSecret:            swag.String(fmt.Sprintf(psTemplate, utils_test.FakePS2)),
					BaseDNSDomain:         "example.com",
					CPUArchitecture:       cpuArchitecture,
					ControlPlaneCount:     swag.Int64(ctrlPlaneCount),
					UserManagedNetworking: userManagedNetworking,
				},
			})

			if errRegisterCluster != nil {
				return nil, errRegisterCluster
			}

			return cluster, nil
		}

		registerNewInfraEnv := func(id *strfmt.UUID, version, cpuArchitecture string) (*installer.RegisterInfraEnvCreated, error) {
			infraEnv, errRegisterInfraEnv := utils_test.TestContext.User2BMClient.Installer.RegisterInfraEnv(context.Background(), &installer.RegisterInfraEnvParams{
				InfraenvCreateParams: &models.InfraEnvCreateParams{
					Name:             swag.String("test-infra-env"),
					OpenshiftVersion: version,
					PullSecret:       swag.String(fmt.Sprintf(psTemplate, utils_test.FakePS2)),
					SSHAuthorizedKey: swag.String(utils_test.SshPublicKey),
					ImageType:        models.ImageTypeFullIso,
					ClusterID:        id,
					CPUArchitecture:  cpuArchitecture,
				},
			})

			return infraEnv, errRegisterInfraEnv
		}

		Context("GetSupportedFeatures", func() {

			It("With Platform", func() {
				features, err := featureSupport("4.14", swag.String("external"), swag.String(models.ClusterCPUArchitectureX8664))
				Expect(err).NotTo(HaveOccurred())

				Expect(funk.Contains(*features, string(models.FeatureSupportLevelIDPLATFORMMANAGEDNETWORKING))).To(Equal(true))
				Expect(funk.Contains(*features, string(models.FeatureSupportLevelIDNUTANIXINTEGRATION))).To(Equal(false))
				Expect(funk.Contains(*features, string(models.FeatureSupportLevelIDEXTERNALPLATFORMOCI))).To(Equal(false))
				Expect(funk.Contains(*features, string(models.FeatureSupportLevelIDBAREMETALPLATFORM))).To(Equal(false))
				Expect(funk.Contains(*features, string(models.FeatureSupportLevelIDNONEPLATFORM))).To(Equal(false))
				Expect(funk.Contains(*features, string(models.FeatureSupportLevelIDVSPHEREINTEGRATION))).To(Equal(false))
			})

			It("Without Platform", func() {
				features, err := featureSupport("4.14", nil, swag.String(models.ClusterCPUArchitectureX8664))
				Expect(err).NotTo(HaveOccurred())

				Expect(funk.Contains(*features, string(models.FeatureSupportLevelIDPLATFORMMANAGEDNETWORKING))).To(Equal(false))
				Expect(funk.Contains(*features, string(models.FeatureSupportLevelIDNUTANIXINTEGRATION))).To(Equal(true))
				Expect(funk.Contains(*features, string(models.FeatureSupportLevelIDEXTERNALPLATFORMOCI))).To(Equal(true))
				Expect(funk.Contains(*features, string(models.FeatureSupportLevelIDBAREMETALPLATFORM))).To(Equal(true))
				Expect(funk.Contains(*features, string(models.FeatureSupportLevelIDNONEPLATFORM))).To(Equal(true))
				Expect(funk.Contains(*features, string(models.FeatureSupportLevelIDVSPHEREINTEGRATION))).To(Equal(true))
			})

		})

		Context("Update cluster", func() {
			It("Update umn true won't fail with multi release without infra-env", func() {
				cluster, err := registerNewCluster(multiarchOpenshiftVersion, common.MultiCPUArchitecture, int64(common.MinMasterHostsNeededForInstallationInHaMode), swag.Bool(true))
				Expect(err).NotTo(HaveOccurred())
				Expect(cluster.Payload.CPUArchitecture).To(Equal(common.MultiCPUArchitecture))

				_, err = utils_test.TestContext.User2BMClient.Installer.V2UpdateCluster(ctx, &installer.V2UpdateClusterParams{
					ClusterUpdateParams: &models.V2ClusterUpdateParams{
						UserManagedNetworking: swag.Bool(false),
					},
					ClusterID: *cluster.Payload.ID,
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("Update umn true fail with s390x with infra-env", func() {
				expectedError := "cannot use Cluster Managed Networking because it's not compatible with the s390x architecture"
				cluster, err := registerNewCluster(multiarchOpenshiftVersion, common.S390xCPUArchitecture, int64(common.MinMasterHostsNeededForInstallationInHaMode), swag.Bool(true))
				Expect(err).NotTo(HaveOccurred())
				Expect(cluster.Payload.CPUArchitecture).To(Equal(common.MultiCPUArchitecture))

				infraEnv, err := registerNewInfraEnv(cluster.Payload.ID, openshiftVersion, "s390x")
				Expect(err).NotTo(HaveOccurred())
				Expect(infraEnv.Payload.CPUArchitecture).To(Equal("s390x"))

				_, err = utils_test.TestContext.User2BMClient.Installer.V2UpdateCluster(ctx, &installer.V2UpdateClusterParams{
					ClusterUpdateParams: &models.V2ClusterUpdateParams{
						UserManagedNetworking: swag.Bool(false),
					},
					ClusterID: *cluster.Payload.ID,
				})
				Expect(err).To(HaveOccurred())
				err2 := err.(*installer.V2UpdateClusterBadRequest)
				ExpectWithOffset(1, *err2.Payload.Reason).To(ContainSubstring(expectedError))
			})

			It("Create infra-env after updating OLM operators on s390x architecture ", func() {
				expectedError := "cannot use OpenShift Virtualization because it's not compatible with the s390x architecture"
				cluster, err := registerNewCluster(multiarchOpenshiftVersion, common.S390xCPUArchitecture, int64(common.MinMasterHostsNeededForInstallationInHaMode), swag.Bool(true))
				Expect(err).NotTo(HaveOccurred())
				Expect(cluster.Payload.CPUArchitecture).To(Equal(common.MultiCPUArchitecture))

				_, err = utils_test.TestContext.User2BMClient.Installer.V2UpdateCluster(ctx, &installer.V2UpdateClusterParams{
					ClusterUpdateParams: &models.V2ClusterUpdateParams{
						OlmOperators: []*models.OperatorCreateParams{
							{Name: "odf"},
							{Name: "cnv"},
							{Name: "mce"},
						},
					},
					ClusterID: *cluster.Payload.ID,
				})
				Expect(err).ToNot(HaveOccurred())

				_, err = registerNewInfraEnv(cluster.Payload.ID, openshiftVersion, "s390x")
				err2 := err.(*installer.RegisterInfraEnvBadRequest)
				ExpectWithOffset(1, *err2.Payload.Reason).To(ContainSubstring(expectedError))
			})
			Context("UpdateInfraEnv", func() {
				It("Update ppc64le infra env minimal iso without cluster", func() {
					infraEnv, err := registerNewInfraEnv(nil, openshiftVersion, models.ClusterCPUArchitecturePpc64le)
					Expect(err).ToNot(HaveOccurred())
					Expect(common.ImageTypeValue(infraEnv.Payload.Type)).ToNot(Equal(models.ImageTypeMinimalIso))

					updatedInfraEnv, err := utils_test.TestContext.User2BMClient.Installer.UpdateInfraEnv(ctx, &installer.UpdateInfraEnvParams{
						InfraEnvID: *infraEnv.Payload.ID,
						InfraEnvUpdateParams: &models.InfraEnvUpdateParams{
							ImageType: models.ImageTypeMinimalIso,
						}})
					Expect(err).ToNot(HaveOccurred())
					Expect(common.ImageTypeValue(updatedInfraEnv.Payload.Type)).To(Equal(models.ImageTypeMinimalIso))
				})
				It("Update ppc64le infra env minimal iso with cluster", func() {
					cluster, err := registerNewCluster(openshiftVersion, models.ClusterCPUArchitecturePpc64le, int64(common.MinMasterHostsNeededForInstallationInHaMode), swag.Bool(true))
					Expect(err).NotTo(HaveOccurred())

					infraEnv, err := registerNewInfraEnv(cluster.Payload.ID, openshiftVersion, models.ClusterCPUArchitecturePpc64le)
					Expect(err).ToNot(HaveOccurred())
					Expect(common.ImageTypeValue(infraEnv.Payload.Type)).ToNot(Equal(models.ImageTypeMinimalIso))

					updatedInfraEnv, err := utils_test.TestContext.User2BMClient.Installer.UpdateInfraEnv(ctx, &installer.UpdateInfraEnvParams{
						InfraEnvID: *infraEnv.Payload.ID,
						InfraEnvUpdateParams: &models.InfraEnvUpdateParams{
							ImageType: models.ImageTypeMinimalIso,
						}})
					Expect(err).ToNot(HaveOccurred())
					Expect(common.ImageTypeValue(updatedInfraEnv.Payload.Type)).To(Equal(models.ImageTypeMinimalIso))
				})
				It("Update s390x infra env minimal iso with cluster - fail", func() {
					cluster, err := registerNewCluster(openshiftVersion, "s390x", int64(common.MinMasterHostsNeededForInstallationInHaMode), swag.Bool(true))
					Expect(err).NotTo(HaveOccurred())

					infraEnv, err := registerNewInfraEnv(cluster.Payload.ID, openshiftVersion, models.ClusterCPUArchitectureS390x)
					Expect(err).ToNot(HaveOccurred())
					Expect(common.ImageTypeValue(infraEnv.Payload.Type)).ToNot(Equal(models.ImageTypeMinimalIso))

					_, err = utils_test.TestContext.User2BMClient.Installer.UpdateInfraEnv(ctx, &installer.UpdateInfraEnvParams{
						InfraEnvID: *infraEnv.Payload.ID,
						InfraEnvUpdateParams: &models.InfraEnvUpdateParams{
							ImageType: models.ImageTypeMinimalIso,
						}})
					Expect(err).To(HaveOccurred())
					err2 := err.(*installer.UpdateInfraEnvBadRequest)
					ExpectWithOffset(1, *err2.Payload.Reason).To(ContainSubstring("cannot use Minimal ISO because it's not compatible with the s390x architecture"))
				})
			})
		})

		Context("Register cluster", func() {
			It("Register cluster won't fail with s390x", func() {
				cluster, err := registerNewCluster(openshiftVersion, "s390x", int64(common.MinMasterHostsNeededForInstallationInHaMode), swag.Bool(true))
				Expect(err).NotTo(HaveOccurred())
				Expect(cluster.Payload.CPUArchitecture).To(Equal(common.S390xCPUArchitecture))
			})

			It("Register cluster won't fail with s390x without UMN", func() {
				cluster, err := registerNewCluster(openshiftVersion, "s390x", int64(common.MinMasterHostsNeededForInstallationInHaMode), nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(cluster.Payload.CPUArchitecture).To(Equal(common.S390xCPUArchitecture))
			})
			It("SNO with s390x won't fail on SNO", func() {
				cluster, err := registerNewCluster(openshiftVersion, "s390x", int64(1), nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(cluster.Payload.CPUArchitecture).To(Equal(common.S390xCPUArchitecture))
				Expect(cluster.Payload.ControlPlaneCount).To(Equal(int64(1)))

			})
		}) // Register cluster

		Context("Supported features", func() {
			availableVersions := []string{"4.8", "4.9", "4.10", "4.11", "4.12", "4.13"}

			var params installer.GetSupportedFeaturesParams
			arch := models.ClusterCPUArchitectureX8664

			for _, v := range availableVersions {
				version := v

				It(fmt.Sprintf("GetSupportedFeatures x86 CPU architectrue, OCP version %s", version), func() {
					params = installer.GetSupportedFeaturesParams{
						OpenshiftVersion: version,
						CPUArchitecture:  swag.String(arch),
					}
					response, err := utils_test.TestContext.UserBMClient.Installer.GetSupportedFeatures(ctx, &params)
					Expect(err).ShouldNot(HaveOccurred())

					for featureID, supportLevel := range response.Payload.Features {
						filters := featuresupport.SupportLevelFilters{OpenshiftVersion: version, CPUArchitecture: swag.String(arch)}
						featureSupportLevel := featuresupport.GetSupportLevel(models.FeatureSupportLevelID(featureID), filters)
						Expect(featureSupportLevel).To(BeEquivalentTo(supportLevel))
					}
				})

				It(fmt.Sprintf("GetSupportedFeatures with empty CPU architectrue, OCP version %s", version), func() {
					response, err := utils_test.TestContext.UserBMClient.Installer.GetSupportedFeatures(ctx, &installer.GetSupportedFeaturesParams{OpenshiftVersion: version})
					Expect(err).ShouldNot(HaveOccurred())

					for featureID, supportLevel := range response.Payload.Features {
						filters := featuresupport.SupportLevelFilters{OpenshiftVersion: version, CPUArchitecture: swag.String(common.DefaultCPUArchitecture)}
						featureSupportLevel := featuresupport.GetSupportLevel(models.FeatureSupportLevelID(featureID), filters)
						Expect(featureSupportLevel).To(BeEquivalentTo(supportLevel))
					}
				})
			}
		})

		Context("Supported architectures", func() {
			var params installer.GetSupportedArchitecturesParams

			It("GetSupportedArchitectures with OCP version 4.6", func() {
				version := "4.6"
				params.OpenshiftVersion = version

				response, err := utils_test.TestContext.UserBMClient.Installer.GetSupportedArchitectures(ctx, &params)
				Expect(err).ShouldNot(HaveOccurred())

				architecturesSupportLevel := response.Payload.Architectures

				Expect(architecturesSupportLevel[string(models.ArchitectureSupportLevelIDX8664ARCHITECTURE)]).To(BeEquivalentTo(models.SupportLevelSupported))
				Expect(architecturesSupportLevel[string(models.ArchitectureSupportLevelIDARM64ARCHITECTURE)]).To(BeEquivalentTo(models.SupportLevelUnavailable))
				Expect(architecturesSupportLevel[string(models.ArchitectureSupportLevelIDS390XARCHITECTURE)]).To(BeEquivalentTo(models.SupportLevelUnavailable))
				Expect(architecturesSupportLevel[string(models.ArchitectureSupportLevelIDPPC64LEARCHITECTURE)]).To(BeEquivalentTo(models.SupportLevelUnavailable))
				Expect(architecturesSupportLevel[string(models.ArchitectureSupportLevelIDMULTIARCHRELEASEIMAGE)]).To(BeEquivalentTo(models.SupportLevelUnavailable))
			})

			It("GetSupportedArchitectures with OCP version 4.12", func() {
				version := "4.12"
				params.OpenshiftVersion = version

				response, err := utils_test.TestContext.UserBMClient.Installer.GetSupportedArchitectures(ctx, &params)
				Expect(err).ShouldNot(HaveOccurred())

				architecturesSupportLevel := response.Payload.Architectures

				Expect(architecturesSupportLevel[string(models.ArchitectureSupportLevelIDX8664ARCHITECTURE)]).To(BeEquivalentTo(models.SupportLevelSupported))
				Expect(architecturesSupportLevel[string(models.ArchitectureSupportLevelIDARM64ARCHITECTURE)]).To(BeEquivalentTo(models.SupportLevelSupported))
				Expect(architecturesSupportLevel[string(models.ArchitectureSupportLevelIDS390XARCHITECTURE)]).To(BeEquivalentTo(models.SupportLevelTechPreview))
				Expect(architecturesSupportLevel[string(models.ArchitectureSupportLevelIDPPC64LEARCHITECTURE)]).To(BeEquivalentTo(models.SupportLevelTechPreview))
				Expect(architecturesSupportLevel[string(models.ArchitectureSupportLevelIDMULTIARCHRELEASEIMAGE)]).To(BeEquivalentTo(models.SupportLevelTechPreview))
			})

			It("GetSupportedArchitectures with OCP version 4.13", func() {
				version := "4.13"
				params.OpenshiftVersion = version

				response, err := utils_test.TestContext.UserBMClient.Installer.GetSupportedArchitectures(ctx, &params)
				Expect(err).ShouldNot(HaveOccurred())

				architecturesSupportLevel := response.Payload.Architectures

				Expect(architecturesSupportLevel[string(models.ArchitectureSupportLevelIDX8664ARCHITECTURE)]).To(BeEquivalentTo(models.SupportLevelSupported))
				Expect(architecturesSupportLevel[string(models.ArchitectureSupportLevelIDARM64ARCHITECTURE)]).To(BeEquivalentTo(models.SupportLevelSupported))
				Expect(architecturesSupportLevel[string(models.ArchitectureSupportLevelIDS390XARCHITECTURE)]).To(BeEquivalentTo(models.SupportLevelSupported))
				Expect(architecturesSupportLevel[string(models.ArchitectureSupportLevelIDPPC64LEARCHITECTURE)]).To(BeEquivalentTo(models.SupportLevelSupported))
				Expect(architecturesSupportLevel[string(models.ArchitectureSupportLevelIDMULTIARCHRELEASEIMAGE)]).To(BeEquivalentTo(models.SupportLevelTechPreview))
			})
		})

	})
})
