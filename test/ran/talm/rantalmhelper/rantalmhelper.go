package rantalmhelper

import (
	"context"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
)

var TalmDynamicClient dynamic.Interface

// GetTestContext fetches a k8s context object for the talm tests.
func GetTestContext() context.Context {
	// Not really sure whether this should be background or todo but this seems to be working fine
	return context.Background()
}

// ConvertYamlObjectToUnstructured is used to convert raw yaml bytes into a k8s unstructed object.
func ConvertYamlObjectToUnstructured(rawYaml []byte) (unstructured.Unstructured, error) {
	// Create an empty object
	result := &unstructured.Unstructured{}

	// Get a yaml decoder
	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	// Decode the raw yaml
	_, _, err := decoder.Decode(rawYaml, nil, result)

	// Check for error
	if err != nil {
		return *result, err
	}

	// Return the final result
	return *result, nil
}

// GetGvtFromUnstructured returns a group version resource that can be used alongside the unstructured object.
func GetGvrFromUnstructured(object unstructured.Unstructured) schema.GroupVersionResource {
	// This can mostly be populated based on the object's GVK
	gvr := schema.GroupVersionResource{
		Group:    object.GetObjectKind().GroupVersionKind().Group,
		Version:  object.GetObjectKind().GroupVersionKind().Version,
		Resource: object.GetKind(),
	}

	// However to use the GVR with the API we need to use the plural resources name
	if object.GetKind() == "ClusterGroupUpgrade" {
		gvr.Resource = "clustergroupupgrades"
	}

	// Return the final result
	return gvr
}

// ApplyTalmResource can be used to apply a raw yaml object to TALM.
// This can probably be replaced with the codegen client when it is finished.
func ApplyTalmResource(contentRawYaml []byte) (unstructured.Unstructured, error) {
	// Serialize the YAML content to an unstructed object
	object, err := ConvertYamlObjectToUnstructured(contentRawYaml)
	if err != nil {
		return object, err
	}

	// Build some context objects that will be required soon
	ctx := GetTestContext()
	gvr := GetGvrFromUnstructured(object)

	// Check if the object exists
	_, err = TalmDynamicClient.
		Resource(gvr).
		Namespace(object.GetNamespace()).
		Get(ctx, object.GetName(), metav1.GetOptions{})

	// This will be non-nil if the object does not exist
	if err != nil {
		// Check if the error is the one we were expecting
		if strings.Contains(err.Error(), "not found") {
			// Create new object
			result, err := TalmDynamicClient.
				Resource(gvr).
				Namespace(object.GetNamespace()).
				Create(ctx, &object, metav1.CreateOptions{
					FieldValidation: metav1.FieldValidationStrict,
				})

			return *result, err
		}

		return object, err
	}

	// Update existing object
	result, err := TalmDynamicClient.
		Resource(gvr).
		Namespace(object.GetNamespace()).
		Update(ctx, &object, metav1.UpdateOptions{
			FieldValidation: metav1.FieldValidationStrict,
		})

	return *result, err
}

// DeleteTalmResource can be used to delete a raw yaml object from TALM.
// This can probably be replaced with the codegen client when it is finished.
func DeleteTalmResource(contentRawYaml []byte) error {
	// Serialize the YAML content to an unstructed object
	object, err := ConvertYamlObjectToUnstructured(contentRawYaml)
	if err != nil {
		return err
	}

	// Build some context objects that will be required soon
	ctx := GetTestContext()
	gvr := GetGvrFromUnstructured(object)

	// Check if the object exists
	err = TalmDynamicClient.
		Resource(gvr).
		Namespace(object.GetNamespace()).
		Delete(ctx, object.GetName(), metav1.DeleteOptions{})
	// This will be non-nil if the object does not exist
	if err != nil {
		// Check if the error is the one we were expecting
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
	}

	return err
}

// GetTalmCondition can be used to obtain a specific condition from a CGU.
// This can probably be replaced with the codegen client when it is finished.
func GetTalmCondition(object unstructured.Unstructured, conditionType string) (metav1.Condition, error) {
	// Build some context objects that will be required soon
	ctx := GetTestContext()
	gvr := GetGvrFromUnstructured(object)

	// Check if the object exists
	result, err := TalmDynamicClient.
		Resource(gvr).
		Namespace(object.GetNamespace()).
		Get(ctx, object.GetName(), metav1.GetOptions{})
	if err != nil {
		return metav1.Condition{}, err
	}

	// Get the conditions from the nested object
	conditions, ok, err := unstructured.NestedSlice(result.Object, "status", "conditions")
	if !ok || err != nil {
		return metav1.Condition{}, err
	}

	// Loop over all the conditions
	for _, condition := range conditions {
		// If the condition matched the type we are looking for then return it
		convertedType, convertedTypeOk := condition.(map[string]interface{})["type"].(string)
		if convertedTypeOk {
			if convertedType == conditionType {
				// The interface must be converted back to a condition before we can return it
				// Most of the elements are just strings and can be done in line
				converted := metav1.Condition{}
				converted.Type = convertedType

				convertedReason, convertedReasonOk := condition.(map[string]interface{})["reason"].(string)
				if convertedReasonOk {
					converted.Reason = convertedReason
				}

				convertedMessage, convertedMessageOk := condition.(map[string]interface{})["message"].(string)
				if convertedMessageOk {
					converted.Message = convertedMessage
				}

				convertedStatus, convertedStatusOk := condition.(map[string]interface{})["status"].(string)
				if convertedStatusOk {
					// The condition status must be converted to a metav1.ConditionStatus object
					switch convertedStatus {
					case "true":
						converted.Status = metav1.ConditionTrue
					case "false":
						converted.Status = metav1.ConditionFalse
					default:
						converted.Status = metav1.ConditionUnknown
					}
				}

				// The timestamp must be parsed back and converted to a metav1.Time object
				convertedTime, convertedTimeOk := condition.(map[string]interface{})["lastTransitionTime"].(string)
				if convertedTimeOk {
					time, err := time.Parse(time.RFC3339, convertedTime)
					if err != nil {
						converted.LastTransitionTime = metav1.NewTime(time)
					}
				}

				// Finally return the converted object
				return converted, nil
			}
		}
	}

	return metav1.Condition{}, nil
}
