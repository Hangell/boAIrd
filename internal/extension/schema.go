package extension

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	maxSchemaBytes = 32 << 10
	maxSchemaDepth = 16
)

var allowedSchemaKeywords = map[string]bool{
	"type": true, "properties": true, "required": true,
	"additionalProperties": true, "items": true, "enum": true,
	"minLength": true, "maxLength": true, "minItems": true,
	"maxItems": true, "minimum": true, "maximum": true, "pattern": true,
}

type jsonSchema struct {
	typeName             string
	properties           map[string]jsonSchema
	required             []string
	additionalProperties *bool
	items                *jsonSchema
	enum                 []any
	minLength            *int
	maxLength            *int
	minItems             *int
	maxItems             *int
	minimum              *float64
	maximum              *float64
	pattern              *regexp.Regexp
}

// ValidateJSONSchema accepts the bounded JSON Schema subset supported by SDK v1.
func ValidateJSONSchema(schema, value json.RawMessage) error {
	compiled, err := compileSchema(schema, 0)
	if err != nil {
		return err
	}
	if len(value) == 0 || len(value) > 64<<10 {
		return errors.New("JSON value is empty or exceeds the schema input limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return errors.New("JSON value is invalid")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("JSON value contains trailing data")
	}
	if err := compiled.validate(decoded); err != nil {
		return errors.New("JSON value does not match the declared schema")
	}
	return nil
}

func validSchema(schema json.RawMessage) bool {
	compiled, err := compileSchema(schema, 0)
	return err == nil && compiled.typeName == "object"
}

func compileSchema(raw json.RawMessage, depth int) (jsonSchema, error) {
	if len(raw) == 0 || len(raw) > maxSchemaBytes || depth > maxSchemaDepth {
		return jsonSchema{}, errors.New("schema is empty, too large, or too deeply nested")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return jsonSchema{}, errors.New("schema must be a JSON object")
	}
	for name := range fields {
		if !allowedSchemaKeywords[name] {
			return jsonSchema{}, fmt.Errorf("unsupported schema keyword %q", name)
		}
	}
	var result jsonSchema
	if err := json.Unmarshal(fields["type"], &result.typeName); err != nil {
		return jsonSchema{}, errors.New("schema type is required")
	}
	switch result.typeName {
	case "object", "array", "string", "number", "integer", "boolean", "null":
	default:
		return jsonSchema{}, errors.New("schema type is unsupported")
	}
	if rawProperties, exists := fields["properties"]; exists {
		if result.typeName != "object" {
			return jsonSchema{}, errors.New("properties require object type")
		}
		var properties map[string]json.RawMessage
		if err := json.Unmarshal(rawProperties, &properties); err != nil || properties == nil || len(properties) > 128 {
			return jsonSchema{}, errors.New("schema properties must be a bounded object")
		}
		result.properties = make(map[string]jsonSchema, len(properties))
		for name, property := range properties {
			compiled, err := compileSchema(property, depth+1)
			if err != nil {
				return jsonSchema{}, fmt.Errorf("property %q: %w", name, err)
			}
			result.properties[name] = compiled
		}
	}
	if rawRequired, exists := fields["required"]; exists {
		if result.typeName != "object" || json.Unmarshal(rawRequired, &result.required) != nil || len(result.required) > 128 {
			return jsonSchema{}, errors.New("required must be a bounded array on object schemas")
		}
		seen := make(map[string]struct{}, len(result.required))
		for _, name := range result.required {
			if _, ok := result.properties[name]; !ok {
				return jsonSchema{}, errors.New("required field must have a property schema")
			}
			if _, ok := seen[name]; ok {
				return jsonSchema{}, errors.New("required fields must be unique")
			}
			seen[name] = struct{}{}
		}
	}
	if rawAdditional, exists := fields["additionalProperties"]; exists {
		if result.typeName != "object" || json.Unmarshal(rawAdditional, &result.additionalProperties) != nil || result.additionalProperties == nil {
			return jsonSchema{}, errors.New("additionalProperties must be a boolean on object schemas")
		}
	}
	if rawItems, exists := fields["items"]; exists {
		if result.typeName != "array" {
			return jsonSchema{}, errors.New("items require array type")
		}
		compiled, err := compileSchema(rawItems, depth+1)
		if err != nil {
			return jsonSchema{}, fmt.Errorf("array items: %w", err)
		}
		result.items = &compiled
	}
	if result.typeName == "array" && result.items == nil {
		return jsonSchema{}, errors.New("array schemas require an items schema")
	}
	if rawEnum, exists := fields["enum"]; exists {
		decoder := json.NewDecoder(bytes.NewReader(rawEnum))
		decoder.UseNumber()
		if err := decoder.Decode(&result.enum); err != nil || len(result.enum) == 0 || len(result.enum) > 128 {
			return jsonSchema{}, errors.New("enum must contain between 1 and 128 values")
		}
	}
	if err := decodeBound(fields, "minLength", result.typeName == "string", &result.minLength); err != nil {
		return jsonSchema{}, err
	}
	if err := decodeBound(fields, "maxLength", result.typeName == "string", &result.maxLength); err != nil {
		return jsonSchema{}, err
	}
	if err := decodeBound(fields, "minItems", result.typeName == "array", &result.minItems); err != nil {
		return jsonSchema{}, err
	}
	if err := decodeBound(fields, "maxItems", result.typeName == "array", &result.maxItems); err != nil {
		return jsonSchema{}, err
	}
	if result.minLength != nil && result.maxLength != nil && *result.minLength > *result.maxLength ||
		result.minItems != nil && result.maxItems != nil && *result.minItems > *result.maxItems {
		return jsonSchema{}, errors.New("schema minimum length exceeds maximum length")
	}
	if err := decodeNumberBound(fields, "minimum", result.typeName, &result.minimum); err != nil {
		return jsonSchema{}, err
	}
	if err := decodeNumberBound(fields, "maximum", result.typeName, &result.maximum); err != nil {
		return jsonSchema{}, err
	}
	if result.minimum != nil && result.maximum != nil && *result.minimum > *result.maximum {
		return jsonSchema{}, errors.New("schema minimum exceeds maximum")
	}
	if rawPattern, exists := fields["pattern"]; exists {
		if result.typeName != "string" {
			return jsonSchema{}, errors.New("pattern requires string type")
		}
		var pattern string
		if err := json.Unmarshal(rawPattern, &pattern); err != nil || len(pattern) > 256 {
			return jsonSchema{}, errors.New("pattern must be a bounded string")
		}
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return jsonSchema{}, errors.New("pattern is invalid")
		}
		result.pattern = compiled
	}
	return result, nil
}

func decodeBound(fields map[string]json.RawMessage, name string, allowed bool, destination **int) error {
	raw, exists := fields[name]
	if !exists {
		return nil
	}
	if !allowed {
		return fmt.Errorf("%s is not valid for this schema type", name)
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil || value < 0 || value > 1<<20 {
		return fmt.Errorf("%s must be a non-negative bounded integer", name)
	}
	*destination = &value
	return nil
}

func decodeNumberBound(fields map[string]json.RawMessage, name, typeName string, destination **float64) error {
	raw, exists := fields[name]
	if !exists {
		return nil
	}
	if typeName != "number" && typeName != "integer" {
		return fmt.Errorf("%s is not valid for this schema type", name)
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%s must be a finite number", name)
	}
	*destination = &value
	return nil
}

func (s jsonSchema) validate(value any) error {
	if s.enum != nil && !slices.ContainsFunc(s.enum, func(candidate any) bool { return equalJSON(candidate, value) }) {
		return errors.New("value is not in enum")
	}
	switch s.typeName {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return errors.New("expected object")
		}
		for _, name := range s.required {
			if _, ok := object[name]; !ok {
				return errors.New("required property is missing")
			}
		}
		for name, propertyValue := range object {
			property, exists := s.properties[name]
			if !exists {
				if s.additionalProperties != nil && !*s.additionalProperties {
					return errors.New("additional property is not allowed")
				}
				continue
			}
			if err := property.validate(propertyValue); err != nil {
				return err
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok || s.minItems != nil && len(array) < *s.minItems || s.maxItems != nil && len(array) > *s.maxItems {
			return errors.New("array length is invalid")
		}
		for _, item := range array {
			if err := s.items.validate(item); err != nil {
				return err
			}
		}
	case "string":
		stringValue, ok := value.(string)
		length := utf8.RuneCountInString(stringValue)
		if !ok || s.minLength != nil && length < *s.minLength || s.maxLength != nil && length > *s.maxLength || s.pattern != nil && !s.pattern.MatchString(stringValue) {
			return errors.New("string value is invalid")
		}
	case "number", "integer":
		number, ok := value.(json.Number)
		if !ok {
			return errors.New("expected number")
		}
		parsed, err := number.Float64()
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || s.minimum != nil && parsed < *s.minimum || s.maximum != nil && parsed > *s.maximum {
			return errors.New("number is out of range")
		}
		if s.typeName == "integer" && math.Trunc(parsed) != parsed {
			return errors.New("expected integer")
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return errors.New("expected boolean")
		}
	case "null":
		if value != nil {
			return errors.New("expected null")
		}
	}
	return nil
}

func equalJSON(left, right any) bool {
	leftBytes, leftErr := json.Marshal(left)
	rightBytes, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && strings.TrimSpace(string(leftBytes)) == strings.TrimSpace(string(rightBytes))
}
