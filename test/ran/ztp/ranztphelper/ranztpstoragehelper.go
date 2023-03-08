package ranztphelper

import (
	"fmt"
	"log"
	"strings"
	"time"

	imageregistryv1 "github.com/openshift/api/imageregistry/v1"
	v1 "github.com/openshift/api/operator/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// CleanupImageRegistryConfig is used to cleanup all the configuration related to an image registry configuration.
func CleanupImageRegistryConfig(
	storageClassName,
	persistentVolumeName,
	persistentVolumeClaimName, persistentVolumeClaimNamespace string,
	client *testClient.ClientSet) error {
	log.Println("Cleaning up the resources for image registry configuration")

	// Check if the client is defined
	if client == nil {
		return fmt.Errorf("provided nil client")
	}

	// We must delete the pieces in order or else we will get an error
	// 1. Persistent volume claim
	// 2. Persistent volume
	// 3. Storage class

	// Check if we were given a persistent volume claim
	if persistentVolumeClaimName != "" {
		// Delete the config if it exists
		err := DeleteAndWaitPersistentVolumeClaim(
			persistentVolumeClaimName,
			persistentVolumeClaimNamespace,
			ranztpparameters.ArgocdChangeTimeout,
			client,
		)
		if err != nil {
			return err
		}
	}

	// Check if we were given a persistent volume
	if persistentVolumeName != "" {
		// Delete the pv if it exists
		err := DeleteAndWaitPersistentVolume(
			persistentVolumeName,
			ranztpparameters.ArgocdChangeTimeout,
			client,
		)
		if err != nil {
			return err
		}
	}

	// Check if we were given a storage class
	if storageClassName != "" {
		// Delete the sc if it exists
		err := DeleteAndWaitStorageClass(
			storageClassName,
			ranztpparameters.ArgocdChangeTimeout,
			client,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

//----------------------------
// ImageRegistryconfig helpers
//----------------------------

// GetImageRegistryConfig is used to get the specified image registry config.
func GetImageRegistryConfig(registryConfigName string, client *testClient.ClientSet) (*imageregistryv1.Config, error) {
	// Check if test client is defined
	if client == nil {
		return &imageregistryv1.Config{}, fmt.Errorf("provided nil client")
	}

	// Get the registry config from the client
	imageRegistryConfig, err := client.
		ImageregistryV1Interface.
		Configs().
		Get(
			GetZtpContext(),
			registryConfigName,
			metav1.GetOptions{},
		)

	// Regardless of whether an error occurred or not we want to return both of these
	return imageRegistryConfig, err
}

// DoesImageRegistryConfigExist is used to check whether a specified image registry config exists.
func DoesImageRegistryConfigExist(registryConfigName string, client *testClient.ClientSet) (bool, error) {
	// Check if test client is defined
	if client == nil {
		return false, fmt.Errorf("provided nil client")
	}

	// Get the image registry config
	imageRegistryConfig, err := GetImageRegistryConfig(registryConfigName, client)

	// If it wasn't found then specifically return nil here
	if err != nil && strings.Contains(err.Error(), "not found") {
		return false, nil
	}

	// If there was an error then the name would be empty on the returned config instead of matching
	return imageRegistryConfig.Name == registryConfigName, err
}

// RestoreImageRegistryConfig is used to restore/update an image registry config.
func RestoreImageRegistryConfig(
	registryConfigName string,
	registryConfig *imageregistryv1.Config,
	client *testClient.ClientSet) error {
	var imageRegistryConfig *imageregistryv1.Config

	// Check if test client is defined
	if client == nil {
		return fmt.Errorf("provided nil client")
	}

	// Request timeout may happen to the image registry config CR when the config is processing,
	// get the current image registry config in a poll function so it can retry if timeout
	err := wait.PollImmediate(
		ranztpparameters.ArgocdChangeInterval,
		ranztpparameters.ArgocdChangeTimeout,
		func() (done bool, err error) {
			imageRegistryConfig, err = GetImageRegistryConfig(registryConfigName, client)
			if err != nil {
				if errors.IsNotFound(err) {
					return true, err
				}

				return false, nil
			}

			return true, nil
		},
	)
	if err != nil {
		return err
	}

	log.Printf("Restoring the image registry config '%s'\n", registryConfigName)

	if imageRegistryConfig.GetAnnotations() != nil {
		imageRegistryConfig.SetAnnotations(registryConfig.Annotations)
	}

	if imageRegistryConfig.GetLabels() != nil {
		imageRegistryConfig.SetLabels(imageRegistryConfig.Labels)
	}

	imageRegistryConfig.Spec = registryConfig.Spec

	_, err = client.ImageregistryV1Interface.Configs().Update(
		GetZtpContext(),
		imageRegistryConfig,
		metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	// Wait for the image registry config becomes available
	err = WaitForConditionInImageRegistryConfig(registryConfigName, "Available", "Removed", "True", client)
	if err != nil {
		return err
	}

	return nil
}

// WaitForConditionInImageRegistryConfig is used to wait for an image registry config to reach certain state.
func WaitForConditionInImageRegistryConfig(
	registryConfigName string,
	conditionType, conditionReason string,
	conditionStatus v1.ConditionStatus,
	client *testClient.ClientSet) error {
	log.Printf("Waiting until image registry config \"%s\" status condition '%s' is '%s' with reason '%s'\n",
		registryConfigName,
		conditionType,
		conditionStatus,
		conditionReason,
	)

	err := wait.PollImmediate(
		ranztpparameters.ArgocdChangeInterval,
		ranztpparameters.ArgocdChangeTimeout,
		func() (done bool, err error) {
			config, err := client.
				ImageregistryV1Interface.Configs().Get(
				GetZtpContext(),
				registryConfigName,
				metav1.GetOptions{},
			)
			if err != nil {
				if errors.IsNotFound(err) {
					return false, err
				}

				log.Printf("Failed to get the image registry configs %s, Error: %s. Retrying ...\n",
					registryConfigName, err.Error())

				return false, nil
			}

			// Loop over all the conditions
			for _, condition := range config.Status.Conditions {
				if condition.Type == conditionType {
					if condition.Status == conditionStatus && condition.Reason == conditionReason {
						return true, nil
					}
					// Stop looping the rest conditions if expected type found
					return false, nil
				}
			}

			return false, nil
		},
	)

	return err
}

// DeleteAndWaitImageRegistryConfig is used to delete an image registry config and wait for it to be gone.
func DeleteAndWaitImageRegistryConfig(
	registryConfigName string,
	timeout time.Duration,
	client *testClient.ClientSet) error {
	// Check if test client is defined
	if client == nil {
		return fmt.Errorf("provided nil client")
	}

	// Check if the image registry exists
	exists, err := DoesImageRegistryConfigExist(registryConfigName, client)
	if err != nil {
		return err
	}

	if exists {
		// Delete the config
		log.Printf("Deleting image registry config '%s'\n", registryConfigName)

		err = client.
			ImageregistryV1Interface.
			Configs().
			Delete(
				GetZtpContext(),
				registryConfigName,
				metav1.DeleteOptions{},
			)
		if err != nil {
			return err
		}

		// Wait until the image registry is gone
		log.Printf("Waiting until image registry config '%s' is gone\n", registryConfigName)

		err = wait.PollImmediate(
			ranztpparameters.ArgocdChangeInterval,
			ranztpparameters.ArgocdChangeTimeout,
			func() (done bool, err error) {
				exists, err := DoesImageRegistryConfigExist(registryConfigName, client)
				if err != nil {
					log.Printf("Failed to get the image registry configs %s, Error: %s. Retrying ...\n",
						registryConfigName, err.Error())

					return false, nil
				}

				return !exists, nil
			},
		)

		// Whether or not this is nil we want to return here to avoid the additional logging below for the non existent case
		return err
	}

	log.Printf("Image registry config '%s' does not exist\n", registryConfigName)

	return nil
}

//----------------------------
// PersistentVolumeClaim helpers
//----------------------------

// GetPersistentVolumeClaim is used to get the specified persistent volume claim.
func GetPersistentVolumeClaim(
	persistentVolumeClaimName, persistentVolumeClaimNamespace string,
	client *testClient.ClientSet) (*corev1.PersistentVolumeClaim, error) {
	// Check if test client is defined
	if client == nil {
		return &corev1.PersistentVolumeClaim{}, fmt.Errorf("provided nil client")
	}

	// Check if the persistentVolumeClaim exists
	persistentVolumeClaim, err := client.PersistentVolumeClaims(persistentVolumeClaimNamespace).Get(
		GetZtpContext(),
		persistentVolumeClaimName,
		metav1.GetOptions{},
	)

	// Regardless of whether an error occurred or not we want to return both of these
	return persistentVolumeClaim, err
}

// DoesPersistentVolumeClaimExist is used to check whether a specified persistent volume claim exists.
func DoesPersistentVolumeClaimExist(
	persistentVolumeClaimName, persistentVolumeClaimNamespace string,
	client *testClient.ClientSet) (bool, error) {
	// Check if test client is defined
	if client == nil {
		return false, fmt.Errorf("provided nil client")
	}

	// Get the image registry config
	persistentVolumeClaim, err := GetPersistentVolumeClaim(
		persistentVolumeClaimName,
		persistentVolumeClaimNamespace,
		client,
	)

	// If it wasn't found then specifically return nil here
	if err != nil && strings.Contains(err.Error(), "not found") {
		return false, nil
	}

	// If there was an error then the name would be empty on the returned config instead of matching
	return persistentVolumeClaim.Name == persistentVolumeClaimName, err
}

// DeleteAndWaitPersistentVolume is used to delete a persistent volume and wait for it to be gone.
func DeleteAndWaitPersistentVolumeClaim(
	persistentVolumeClaimName, persistentVolumeClaimNamespace string,
	timeout time.Duration,
	client *testClient.ClientSet) error {
	// Check if test client is defined
	if client == nil {
		return fmt.Errorf("provided nil client")
	}

	// Check if the image registry exists
	exists, err := DoesPersistentVolumeClaimExist(persistentVolumeClaimName, persistentVolumeClaimNamespace, client)
	if err != nil {
		return err
	}

	if exists {
		// Delete the config
		log.Printf("Deleting persistent volume claim '%s'\n", persistentVolumeClaimName)

		err = client.PersistentVolumeClaims(persistentVolumeClaimNamespace).Delete(
			GetZtpContext(),
			persistentVolumeClaimName,
			metav1.DeleteOptions{},
		)
		if err != nil {
			return err
		}

		// Wait until the image registry is gone
		log.Printf("Waiting until persistent volume claim '%s' is gone\n", persistentVolumeClaimName)

		err = wait.PollImmediate(
			ranztpparameters.ArgocdChangeInterval,
			ranztpparameters.ArgocdChangeTimeout,
			func() (done bool, err error) {
				exists, err := DoesPersistentVolumeClaimExist(persistentVolumeClaimName, persistentVolumeClaimNamespace, client)
				if err != nil {
					log.Printf("Failed to get the PVC %s in the namespace %s, Error: %s. Retrying ...\n",
						persistentVolumeClaimName, persistentVolumeClaimNamespace, err.Error())

					return false, nil
				}

				return !exists, nil
			},
		)

		// Whether or not this is nil we want to return here to avoid the additional logging below for the non existent case
		return err
	}

	log.Printf("Persistent volume claim '%s' does not exist\n", persistentVolumeClaimName)

	return nil
}

//----------------------------
// PersistentVolume helpers
//----------------------------

// GetPersistentVolume is used to get the specified persistent volume.
func GetPersistentVolume(persistentVolumeName string, client *testClient.ClientSet) (*corev1.PersistentVolume, error) {
	// Check if test client is defined
	if client == nil {
		return &corev1.PersistentVolume{}, fmt.Errorf("provided nil client")
	}

	// Check if the pvc exists
	persistentVolume, err := client.PersistentVolumes().Get(
		GetZtpContext(),
		persistentVolumeName,
		metav1.GetOptions{},
	)

	// Regardless of whether an error occurred or not we want to return both of these
	return persistentVolume, err
}

// DoesPersistentVolumeExist is used to check whether a specified persistent volume exists.
func DoesPersistentVolumeExist(persistentVolumeName string, client *testClient.ClientSet) (bool, error) {
	// Check if test client is defined
	if client == nil {
		return false, fmt.Errorf("provided nil client")
	}

	// Get the image registry config
	persistentVolume, err := GetPersistentVolume(persistentVolumeName, client)

	// If it wasn't found then specifically return nil here
	if err != nil && strings.Contains(err.Error(), "not found") {
		return false, nil
	}

	// If there was an error then the name would be empty on the returned config instead of matching
	return persistentVolume.Name == persistentVolumeName, err
}

// DeleteAndWaitPersistentVolume is used to delete a persistent volume and wait for it to be gone.
func DeleteAndWaitPersistentVolume(
	persistentVolumeName string,
	timeout time.Duration,
	client *testClient.ClientSet) error {
	// Check if test client is defined
	if client == nil {
		return fmt.Errorf("provided nil client")
	}

	// Check if the image registry exists
	exists, err := DoesPersistentVolumeExist(persistentVolumeName, client)
	if err != nil {
		return err
	}

	if exists {
		// Delete the config
		log.Printf("Deleting persistent volume '%s'\n", persistentVolumeName)

		err = client.PersistentVolumes().Delete(
			GetZtpContext(),
			persistentVolumeName,
			metav1.DeleteOptions{},
		)
		if err != nil {
			return err
		}

		// Wait until the image registry is gone
		log.Printf("Waiting until persistent volume '%s' is gone\n", persistentVolumeName)

		err = wait.PollImmediate(
			ranztpparameters.ArgocdChangeInterval,
			ranztpparameters.ArgocdChangeTimeout,
			func() (done bool, err error) {
				exists, err := DoesPersistentVolumeExist(persistentVolumeName, client)
				if err != nil {
					log.Printf("Failed to get the PV %s, Error: %s. Retrying ...\n", persistentVolumeName, err.Error())

					return false, nil
				}

				return !exists, nil
			},
		)

		// Whether or not this is nil we want to return here to avoid the additional logging below for the non existent case
		return err
	}

	log.Printf("Persistent volume '%s' does not exist\n", persistentVolumeName)

	return nil
}

//----------------------------
// StorageClass helpers
//----------------------------

// GetStorageClass is used to get the specified storage class.
func GetStorageClass(
	storageClassName string,
	client *testClient.ClientSet) (storagev1.StorageClass, error) {
	// Check if test client is defined
	if client == nil {
		return storagev1.StorageClass{}, fmt.Errorf("provided nil client")
	}

	// We need to use a typed namespace to get a storage class
	scType := types.NamespacedName{}
	scType.Name = storageClassName

	storageClass := storagev1.StorageClass{}
	err := client.Get(GetZtpContext(), scType, &storageClass)

	// Regardless of whether an error occurred or not we want to return both of these
	return storageClass, err
}

// DoesStorageClassExist is used to check whether a specified storage class exists.
func DoesStorageClassExist(storageClassName string, client *testClient.ClientSet) (bool, error) {
	// Check if test client is defined
	if client == nil {
		return false, fmt.Errorf("provided nil client")
	}

	// Get the image registry config
	storageClass, err := GetStorageClass(storageClassName, client)

	// If it wasn't found then specifically return nil here
	if err != nil && errors.IsNotFound(err) {
		return false, nil
	}

	// If there was an error then the name would be empty on the returned config instead of matching
	return storageClass.Name == storageClassName, err
}

// DeleteAndWaitStorageClass is used to delete a persistent volume and wait for it to be gone.
func DeleteAndWaitStorageClass(
	storageClassName string,
	timeout time.Duration,
	client *testClient.ClientSet) error {
	// Check if test client is defined
	if client == nil {
		return fmt.Errorf("provided nil client")
	}

	// Check if the image registry exists
	exists, err := DoesStorageClassExist(storageClassName, client)
	if err != nil {
		return err
	}

	if exists {
		// Delete the config
		log.Printf("Deleting storage class '%s'\n", storageClassName)

		// Get the storage class
		storageClass, err := GetStorageClass(storageClassName, client)
		if err != nil {
			return err
		}

		// Delete the storage class
		err = client.Delete(GetZtpContext(), &storageClass, &runtimeClient.DeleteOptions{})
		if err != nil {
			return err
		}

		// Wait until the image registry is gone
		log.Printf("Waiting until storage class '%s' is gone\n", storageClassName)

		err = wait.PollImmediate(
			ranztpparameters.ArgocdChangeInterval,
			ranztpparameters.ArgocdChangeTimeout,
			func() (done bool, err error) {
				exists, err := DoesStorageClassExist(storageClassName, client)
				if err != nil {
					log.Printf("Failed to get the PV %s, Error: %s. Retrying ...\n", storageClassName, err.Error())

					return false, nil
				}

				return !exists, nil
			},
		)

		// Whether or not this is nil we want to return here to avoid the additional logging below for the non existent case
		return err
	}

	log.Printf("Storage class '%s' does not exist\n", storageClassName)

	return nil
}
