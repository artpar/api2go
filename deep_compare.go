package api2go

import (
	"fmt"
	"reflect"
	"sort"
)

// AreValuesEqual compares two interface{} values
// Returns true if they're equal, false otherwise
func AreValuesEqual(a, b interface{}) bool {
	equal, _ := deepCompareValues(a, b)
	return equal
}

// deepCompareValues compares two interface{} values
// Returns true if they're equal, false otherwise, along with a difference description
func deepCompareValues(a, b interface{}) (bool, string) {
	// Handle nil values
	if a == nil && b == nil {
		return true, ""
	}
	if a == nil || b == nil {
		return false, "One value is nil, the other is not"
	}

	valueA := reflect.ValueOf(a)
	valueB := reflect.ValueOf(b)

	// Check if types match
	if valueA.Type() != valueB.Type() {
		return false, fmt.Sprintf("Type mismatch: %T vs %T", a, b)
	}

	// Handle different types
	switch valueA.Kind() {
	case reflect.Map:
		// Convert to map[string]interface{} if possible
		mapA, okA := a.(map[string]interface{})
		mapB, okB := b.(map[string]interface{})

		if !okA || !okB {
			// Fall back to reflect.DeepEqual for non-string keys
			if !reflect.DeepEqual(a, b) {
				return false, "Maps are not equal (using reflect.DeepEqual)"
			}
			return true, ""
		}

		return deepCompareMap(mapA, mapB)

	case reflect.Slice, reflect.Array:
		// Check if it's a slice of maps with string keys
		if valueA.Len() > 0 && valueA.Index(0).Kind() == reflect.Map {
			// Try to convert to []map[string]interface{}
			var sliceA, sliceB []map[string]interface{}

			// Check if conversion is possible
			canConvert := true
			for i := 0; i < valueA.Len(); i++ {
				if i < valueB.Len() {
					itemA, okA := valueA.Index(i).Interface().(map[string]interface{})
					itemB, okB := valueB.Index(i).Interface().(map[string]interface{})
					if !okA || !okB {
						canConvert = false
						break
					}
					sliceA = append(sliceA, itemA)
					sliceB = append(sliceB, itemB)
				}
			}

			if canConvert {
				return DeepCompare(sliceA, sliceB)
			}
		}

		// Check length
		if valueA.Len() != valueB.Len() {
			return false, fmt.Sprintf("Slice length mismatch: %d vs %d", valueA.Len(), valueB.Len())
		}

		// Compare each element
		for i := 0; i < valueA.Len(); i++ {
			equal, diff := deepCompareValues(valueA.Index(i).Interface(), valueB.Index(i).Interface())
			if !equal {
				return false, fmt.Sprintf("Difference at index %d: %s", i, diff)
			}
		}

		return true, ""

	case reflect.Struct:
		// Compare struct fields
		for i := 0; i < valueA.NumField(); i++ {
			fieldA := valueA.Field(i)
			fieldB := valueB.Field(i)

			// Skip unexported fields
			if !fieldA.CanInterface() {
				continue
			}

			equal, diff := deepCompareValues(fieldA.Interface(), fieldB.Interface())
			if !equal {
				fieldName := valueA.Type().Field(i).Name
				return false, fmt.Sprintf("Difference in field '%s': %s", fieldName, diff)
			}
		}

		return true, ""

	case reflect.Ptr:
		// If both are nil, they're equal
		if valueA.IsNil() && valueB.IsNil() {
			return true, ""
		}

		// If one is nil but not the other, they're not equal
		if valueA.IsNil() || valueB.IsNil() {
			return false, "One pointer is nil, the other is not"
		}

		// Compare the values they point to
		return deepCompareValues(valueA.Elem().Interface(), valueB.Elem().Interface())

	default:
		// For primitive types and others, use reflect.DeepEqual
		if !reflect.DeepEqual(a, b) {
			return false, fmt.Sprintf("Values not equal: %v vs %v", a, b)
		}
		return true, ""
	}
}

// deepCompareMap compares two maps of type map[string]interface{}
func deepCompareMap(a, b map[string]interface{}) (bool, string) {
	// Check if both are nil or both are not nil
	if (a == nil) != (b == nil) {
		return false, "One map is nil, the other is not"
	}

	// If both nil, they're equal
	if a == nil && b == nil {
		return true, ""
	}

	// Check length
	if len(a) != len(b) {
		return false, fmt.Sprintf("Map length mismatch: %d vs %d", len(a), len(b))
	}

	// Get all keys from map a
	keys := make([]string, 0, len(a))
	for k := range a {
		keys = append(keys, k)
	}

	// Sort keys for deterministic comparison
	sort.Strings(keys)

	// Check each key
	for _, key := range keys {
		// Check if key exists in b
		valueB, existsB := b[key]
		if !existsB {
			return false, fmt.Sprintf("Key '%s' exists in first map but not in second", key)
		}

		// Check if values are equal
		valueA := a[key]
		equal, diff := deepCompareValues(valueA, valueB)
		if !equal {
			return false, fmt.Sprintf("Value mismatch for key '%s': %s", key, diff)
		}
	}

	return true, ""
}

// DeepCompare performs a deep comparison between two slices of maps
// Returns true if they're equal, false otherwise
// Also returns a string describing the first difference found (empty if equal)
func DeepCompare(a, b []map[string]interface{}) (bool, string) {
	// Check if both are nil or both are not nil
	if (a == nil) != (b == nil) {
		return false, "One slice is nil, the other is not"
	}

	// If both nil, they're equal
	if a == nil && b == nil {
		return true, ""
	}

	// Check length
	if len(a) != len(b) {
		return false, fmt.Sprintf("Length mismatch: %d vs %d", len(a), len(b))
	}

	// Compare each map
	for i := 0; i < len(a); i++ {
		equal, diff := deepCompareMap(a[i], b[i])
		if !equal {
			return false, fmt.Sprintf("Difference at index %d: %s", i, diff)
		}
	}

	return true, ""
}
