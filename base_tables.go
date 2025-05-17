package api2go

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/artpar/api2go/jsonapi"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"reflect"
	"strings"
)

type TableRelationInterface interface {
	GetSubjectName() string
	GetRelation() string
	GetObjectName() string
}

type TableRelation struct {
	Subject     string
	Object      string
	Relation    string
	SubjectName string
	ObjectName  string
	Columns     []ColumnInfo
}

func (tr *TableRelation) String() string {
	return fmt.Sprintf("[TableRelation] [%v][%v][%v]", tr.GetSubjectName(), tr.GetRelation(), tr.GetObjectName())
}

func (tr *TableRelation) Hash() string {
	return fmt.Sprintf("[%v][%v][%v][%v][%v]", tr.GetSubjectName(), tr.GetRelation(), tr.GetObjectName(), tr.GetSubject(), tr.GetObject())
}

func (tr *TableRelation) GetSubjectName() string {
	if tr.SubjectName == "" {
		tr.SubjectName = tr.Subject + "_id"
	}
	return tr.SubjectName
}

func (tr *TableRelation) GetSubject() string {
	return tr.Subject
}

func (tr *TableRelation) GetJoinTableName() string {
	return tr.Subject + "_" + tr.GetSubjectName() + "_has_" + tr.Object + "_" + tr.GetObjectName()
}

func (tr *TableRelation) GetJoinString() string {

	if tr.Relation == "has_one" {
		return fmt.Sprintf(" %s %s on %s.%s = %s.%s ", tr.GetObject(), tr.GetObjectName(), tr.GetSubject(), tr.GetObjectName(), tr.GetObjectName(), "id")
	} else if tr.Relation == "belongs_to" {
		return fmt.Sprintf(" %s %s on %s.%s = %s.%s ", tr.GetObject(), tr.GetObjectName(), tr.GetSubject(), tr.GetObjectName(), tr.GetObjectName(), "id")
	} else if tr.Relation == "has_many" || tr.Relation == "has_many_and_belongs_to_many" {
		return fmt.Sprintf(" %s %s on      %s.%s = %s.id             join %s %s  on  %s.%s = %s.%s ",
			tr.GetJoinTableName(), tr.GetJoinTableName(),
			tr.GetJoinTableName(), tr.GetSubjectName(),
			tr.GetSubject(),
			tr.GetObject(), tr.GetObjectName(),
			tr.GetJoinTableName(), tr.GetObjectName(),
			tr.GetObjectName(), "id")
	} else {
		log.Errorf("Not implemented join: %v", tr)
	}

	return ""
}

func (tr *TableRelation) GetReverseJoinString() string {

	if tr.Relation == "has_one" {
		return fmt.Sprintf(" %s %s on %s.%s = %s.%s ", tr.GetSubject(), tr.GetSubjectName(), tr.GetSubjectName(), tr.GetObjectName(), tr.GetObject(), "id")
	} else if tr.Relation == "belongs_to" {
		return fmt.Sprintf(" %s %s on %s.%s = %s.%s ", tr.GetSubject(), tr.GetSubjectName(), tr.GetSubjectName(), tr.GetObjectName(), tr.GetObject(), "id")
	} else if tr.Relation == "has_many" {

		//select * from user join user_has_usergroup j1 on j1.user_id = user.id  join usergroup on j1.usergroup_id = usergroup.id
		return fmt.Sprintf(" %s %s on %s.%s = %s.id join %s %s on %s.%s = %s.%s ",
			tr.GetJoinTableName(), tr.GetJoinTableName(),
			tr.GetJoinTableName(), tr.GetObjectName(),
			tr.GetObject(),
			tr.GetSubject(), tr.GetSubjectName(),
			tr.GetJoinTableName(), tr.GetSubjectName(),
			tr.GetSubjectName(), "id")
	} else {
		log.Errorf("Not implemented join: %v", tr)
	}

	return ""
}

func (tr *TableRelation) GetRelation() string {
	return tr.Relation
}

func (tr *TableRelation) GetObjectName() string {
	if tr.ObjectName == "" {
		tr.ObjectName = tr.Object + "_id"
	}
	return tr.ObjectName
}

func (tr *TableRelation) GetObject() string {
	return tr.Object
}

func NewTableRelation(subject, relation, object string) TableRelation {
	return TableRelation{
		Subject:     subject,
		Relation:    relation,
		Object:      object,
		SubjectName: subject + "_id",
		ObjectName:  object + "_id",
	}
}

func NewTableRelationWithNames(subject, subjectName, relation, object, objectName string) TableRelation {
	return TableRelation{
		Subject:     subject,
		Relation:    relation,
		Object:      object,
		SubjectName: subjectName,
		ObjectName:  objectName,
	}
}

type Api2GoModel struct {
	typeName          string
	columns           []ColumnInfo
	columnMap         map[string]ColumnInfo
	defaultPermission int64
	DeleteIncludes    map[string][]string
	relations         []TableRelation
	data              map[string]interface{}
	oldData           map[string]interface{}
	Includes          []jsonapi.MarshalIdentifier
	dirty             bool
}

type DeleteReferenceInfo struct {
	ReferenceRelationName string
	ReferenceId           string
}

func (g Api2GoModel) GetNextVersion() int64 {
	if g.dirty {
		return g.oldData["version"].(int64) + 1
	} else {
		version, ok := g.data["version"]
		if !ok {
		}
		return version.(int64) + 1
	}
}

func (g Api2GoModel) HasVersion() bool {
	ok := false
	if !g.dirty {
		_, ok = g.data["version"]
	} else {
		_, ok = g.oldData["version"]
	}
	return ok
}

func (g Api2GoModel) GetCurrentVersion() int64 {
	if g.dirty {
		return g.oldData["version"].(int64)
	} else {
		return g.data["version"].(int64)
	}
}

func (a Api2GoModel) GetColumnMap() map[string]ColumnInfo {
	if a.columnMap != nil && len(a.columnMap) > 0 {
		return a.columnMap
	}

	m := make(map[string]ColumnInfo)

	for _, col := range a.columns {
		m[col.ColumnName] = col
	}
	a.columnMap = m
	return m
}

func (a Api2GoModel) HasColumn(colName string) bool {
	for _, col := range a.columns {
		if col.ColumnName == colName {
			return true
		}
	}

	for _, rel := range a.relations {

		if rel.GetRelation() == "belongs_to" && rel.GetObjectName() == colName {
			return true
		}

	}
	return false
}

func (a Api2GoModel) HasMany(colName string) bool {

	if a.typeName == "usergroup" {
		return false
	}

	for _, rel := range a.relations {
		if rel.GetRelation() == "has_many" && rel.GetObject() == colName {
			//log.Infof("Found %v relation: %v", colName, rel)
			return true
		}
	}
	return false
}

func (a Api2GoModel) GetRelations() []TableRelation {
	return a.relations
}

func (a *Api2GoModel) SetRelations(relations []TableRelation) {
	a.relations = relations
}

type ValueOptions struct {
	ValueType string
	Value     interface{}
	Label     string
}

type ColumnInfo struct {
	Name              string         `db:"name"`
	ColumnName        string         `db:"column_name"`
	ColumnDescription string         `db:"column_description"`
	ColumnType        string         `db:"column_type"`
	IsPrimaryKey      bool           `db:"is_primary_key"`
	IsAutoIncrement   bool           `db:"is_auto_increment"`
	IsIndexed         bool           `db:"is_indexed"`
	IsUnique          bool           `db:"is_unique"`
	IsNullable        bool           `db:"is_nullable"`
	Permission        uint64         `db:"permission"`
	IsForeignKey      bool           `db:"is_foreign_key"`
	ExcludeFromApi    bool           `db:"include_in_api"`
	ForeignKeyData    ForeignKeyData `db:"foreign_key_data"`
	DataType          string         `db:"data_type"`
	DefaultValue      string         `db:"default_value"`
	Options           []ValueOptions
}

type ForeignKeyData struct {
	DataSource string
	Namespace  string
	KeyName    string
}

// Parse format "namespace:tableName(column)"
func (f *ForeignKeyData) Scan(src interface{}) error {
	strValue, ok := src.([]uint8)
	if !ok {
		return fmt.Errorf("metas field must be a string, got %T instead", src)
	}

	parts := strings.Split(string(strValue), "(")
	tableName := parts[0]
	columnName := strings.Split(parts[1], ")")[0]

	dataSource := "self"

	indexColon := strings.Index(tableName, ":")
	if indexColon > -1 {
		dataSource = tableName[0:indexColon]
		tableName = tableName[indexColon+1:]
	}

	f.DataSource = dataSource
	f.KeyName = columnName
	f.Namespace = tableName
	return nil
}

func (f ForeignKeyData) String() string {
	return fmt.Sprintf("%s(%s)", f.Namespace, f.KeyName)
}

// transformNumbersDict walks a json decoded tree an replaces json.Number
// with int64, float64, or string, in this order of preference (i.e. if it
// parses as an int, use int. if it parses as a float, use float. etc).
func transformNumbersDict(dict map[string]interface{}) {
	for k, v := range dict {
		switch vv := v.(type) {
		case json.Number:
			dict[k] = transformNumber(vv)
		case map[string]interface{}:
			transformNumbersDict(vv)
		case []interface{}:
			transformNumbersArray(vv)
		}
	}
}

func transformNumber(value json.Number) interface{} {
	i64, err := value.Int64()
	if err == nil {
		return i64
	}
	f64, err := value.Float64()
	if err == nil {
		return f64
	}
	return value.String()
}

func transformNumbersArray(arr []interface{}) {
	for i, v := range arr {
		switch vv := v.(type) {
		case json.Number:
			arr[i] = transformNumber(vv)
		case map[string]interface{}:
			transformNumbersDict(vv)
		case []interface{}:
			transformNumbersArray(vv)
		}
	}
}

func NewApi2GoModelWithData(
	name string,
	columns []ColumnInfo,
	defaultPermission int64,
	relations []TableRelation,
	m map[string]interface{},
) Api2GoModel {
	if m != nil {
		m["__type"] = name
	}
	transformNumbersDict(m)
	return Api2GoModel{
		typeName:          name,
		columns:           columns,
		relations:         relations,
		data:              m,
		defaultPermission: defaultPermission,
		dirty:             false,
	}
}
func NewApi2GoModel(name string, columns []ColumnInfo, defaultPermission int64, relations []TableRelation) Api2GoModel {
	//fmt.Printf("New columns: %v", columns)
	return Api2GoModel{
		typeName:          name,
		defaultPermission: defaultPermission,
		relations:         relations,
		columns:           columns,
		dirty:             false,
	}
}

func EndsWith(str string, endsWith string) (string, bool) {
	if len(endsWith) > len(str) {
		return "", false
	}

	if len(endsWith) == len(str) && endsWith != str {
		return "", false
	}

	suffix := str[len(str)-len(endsWith):]
	prefix := str[:len(str)-len(endsWith)]

	i := suffix == endsWith
	return prefix, i

}

func EndsWithCheck(str string, endsWith string) bool {
	if len(endsWith) > len(str) {
		return false
	}

	if len(endsWith) == len(str) && endsWith != str {
		return false
	}

	suffix := str[len(str)-len(endsWith):]
	i := suffix == endsWith
	return i

}

func (m *Api2GoModel) SetToOneReferenceID(name, ID string) error {

	if ID == "" {
		return errors.New("referenced id cannot be set to to empty, use delete to remove")
	}
	existingVal, ok := m.data[name]
	if !m.dirty && (!ok || existingVal != ID) {
		m.dirty = true

		tempMap := make(map[string]interface{})

		for k1, v1 := range m.data {
			tempMap[k1] = v1
		}

		m.oldData = tempMap

	}
	m.data[name] = ID
	return nil
}

func (m *Api2GoModel) SetToManyReferenceIDs(name string, IDs []map[string]interface{}) error {

	for _, rel := range m.relations {
		//log.Infof("Check relation: %v", rel.String())
		if rel.GetObjectName() == name || rel.GetSubjectName() == name {
			if rel.GetRelation() == "has_many" || rel.GetRelation() == "has_many_and_belongs_to_many" {

				var rows = make([]map[string]interface{}, 0)
				for _, id := range IDs {
					//row := make(map[string]interface{})
					//row[name] = id
					id["reference_id"] = id["id"]
					if rel.GetSubjectName() == name {
						id[rel.GetObjectName()] = m.data["reference_id"]
					} else {
						id[rel.GetSubjectName()] = m.data["reference_id"]
					}
					rows = append(rows, id)
				}
				if len(rows) > 0 {
					m.data[name] = rows
				}
				return nil
			} else if rel.GetRelation() == "has_one" {

				var rows = make([]map[string]interface{}, 0)
				for _, id := range IDs {
					//row := make(map[string]interface{})
					//row[name] = id
					if rel.GetSubjectName() == name {
						id[rel.GetObjectName()] = m.data["reference_id"]
						id["__type"] = rel.GetSubject()
					} else if rel.GetSubjectName() == name {
						id["__type"] = rel.GetObject()
						id[rel.GetSubjectName()] = m.data["reference_id"]
					}
					rows = append(rows, id)
				}
				//m.SetToOneReferenceID(name, IDs[0])1
				if len(rows) > 0 {
					m.data[name] = rows
				}
				return nil
			}
		}
	}

	return nil

}

// The EditToManyRelations interface can be optionally implemented to add and
// delete to-many relationships on a already unmarshalled struct. These methods
// are used by our API for the to-many relationship update routes.
//
// There are 3 HTTP Methods to edit to-many relations:
//
//	PATCH /v1/posts/1/comments
//	Content-Type: application/vnd.api+json
//	Accept: application/vnd.api+json
//
//	{
//	  "data": [
//		{ "type": "comments", "id": "2" },
//		{ "type": "comments", "id": "3" }
//	  ]
//	}
//
// This replaces all of the comments that belong to post with ID 1 and the
// SetToManyReferenceIDs method will be called.
//
//	POST /v1/posts/1/comments
//	Content-Type: application/vnd.api+json
//	Accept: application/vnd.api+json
//
//	{
//	  "data": [
//		{ "type": "comments", "id": "123" }
//	  ]
//	}
//
// Adds a new comment to the post with ID 1.
// The AddToManyIDs method will be called.
//
//	DELETE /v1/posts/1/comments
//	Content-Type: application/vnd.api+json
//	Accept: application/vnd.api+json
//
//	{
//	  "data": [
//		{ "type": "comments", "id": "12" },
//		{ "type": "comments", "id": "13" }
//	  ]
//	}
//
// Deletes comments that belong to post with ID 1.
// The DeleteToManyIDs method will be called.
type EditToManyRelations interface {
	AddToManyIDs(name string, IDs []string) error
	DeleteToManyIDs(name string, IDs []string) error
}

func (m *Api2GoModel) AddToManyIDs(name string, IDs []string) error {

	new1 := errors.New("There is no to-manyrelationship with the name " + name)
	log.Errorf("ERROR: ", new1)
	return new1
}

func (m *Api2GoModel) DeleteToManyIDs(name string, IDs []string) error {
	log.Infof("set DeleteToManyIDs [%v] == %v", name, IDs)
	referencedRelation := TableRelation{}

	for _, relation := range m.relations {

		if relation.GetSubject() == m.typeName && relation.GetObjectName() == name {
			referencedRelation = relation
			break
		} else if relation.GetObject() == m.typeName && relation.GetSubjectName() == name {
			referencedRelation = relation
			break
		}
	}

	if referencedRelation.GetRelation() == "" {
		return fmt.Errorf("relationship not found: %v", name)
	}

	if (referencedRelation.GetRelation() == "has_one" ||
		referencedRelation.GetRelation() == "belongs_to") &&
		m.typeName == referencedRelation.GetSubject() {
		log.Infof("Has one or belongs to relation")
		if m.data[name] == IDs[0] {
			//m.data[name] = nil
			m.SetAttributes(map[string]interface{}{
				name: nil,
			})
		}
	} else {
		log.Infof("Many to many relation to relation")
		if m.DeleteIncludes == nil {
			m.DeleteIncludes = make(map[string][]string)
		}

		references := m.DeleteIncludes
		references[name] = IDs
		m.DeleteIncludes = references
	}
	log.Infof("New to deletes: %v", m.DeleteIncludes)
	return nil
}

func (m Api2GoModel) GetReferencedStructs() []jsonapi.MarshalIdentifier {
	//log.Infof("References : %v", m.Includes)
	return m.Includes
}

func (m Api2GoModel) GetReferencedIDs() []jsonapi.ReferenceID {

	references := make([]jsonapi.ReferenceID, 0)

	for _, rel := range m.relations {

		//log.Infof("Checked relations [%v]: %v", m.typeName, rel)
		relationType := jsonapi.ToOneRelationship

		if rel.GetRelation() == "belongs_to" || rel.GetRelation() == "has_one" {
			if rel.GetSubject() == m.typeName {

				val, ok := m.data[rel.GetObjectName()]
				if !ok || val == nil {
					continue
				}

				stringValue, ok := val.(string)
				if !ok {
					valStringer, isStringer := val.(fmt.Stringer)
					if isStringer {
						stringValue = valStringer.String()
					} else {
						stringValue = fmt.Sprintf("%v", val)
					}
				}
				ref := jsonapi.ReferenceID{
					Type:         rel.GetObject(),
					Name:         rel.GetObjectName(),
					ID:           stringValue,
					Relationship: relationType,
				}
				references = append(references, ref)
			} else {
				val1, ok := m.data[rel.GetSubjectName()]
				if !ok || val1 == nil {
					continue
				}

				relationType = jsonapi.ToManyRelationship
				valueList := val1.([]string)

				for _, val := range valueList {
					ref := jsonapi.ReferenceID{
						Type:         rel.GetSubject(),
						Name:         rel.GetSubjectName(),
						ID:           val,
						Relationship: relationType,
					}
					references = append(references, ref)
				}

			}

		} else {
			relationType = jsonapi.ToManyRelationship
			if rel.GetSubject() == m.typeName {

				val, ok := m.data[rel.GetObjectName()]
				if !ok || val == nil {
					continue
				}

				valList := val.([]string)
				for _, valId := range valList {
					ref := jsonapi.ReferenceID{
						Type:         rel.GetObject(),
						Name:         rel.GetObjectName(),
						ID:           valId,
						Relationship: relationType,
					}
					references = append(references, ref)
				}

			} else {

				val, ok := m.data[rel.GetSubjectName()]
				if !ok || val == nil {
					continue
				}

				valList := val.([]string)
				for _, valId := range valList {
					ref := jsonapi.ReferenceID{
						Type:         rel.GetSubject(),
						Name:         rel.GetSubjectName(),
						ID:           valId,
						Relationship: relationType,
					}
					references = append(references, ref)
				}

			}
		}

	}

	//log.Infof("Reference ids for %v: %v", m.typeName, references)
	return references

}

func (model Api2GoModel) GetReferences() []jsonapi.Reference {

	references := make([]jsonapi.Reference, 0)
	//

	//log.Infof("Relations: %v", model.relations)
	for _, relation := range model.relations {

		//log.Infof("Check relation [%v] On [%v]", model.typeName, relation.String())
		ref := jsonapi.Reference{}

		if relation.GetSubject() == model.typeName {
			switch relation.GetRelation() {

			case "has_many":
				ref.Type = relation.GetObject()
				ref.Name = relation.GetObjectName()
				ref.Relationship = jsonapi.ToManyRelationship
			case "has_one":
				ref.Type = relation.GetObject()
				ref.Name = relation.GetObjectName()
				ref.Relationship = jsonapi.ToOneRelationship

			case "belongs_to":
				ref.Type = relation.GetObject()
				ref.Name = relation.GetObjectName()
				ref.Relationship = jsonapi.ToOneRelationship
			case "has_many_and_belongs_to_many":
				ref.Type = relation.GetObject()
				ref.Name = relation.GetObjectName()
				ref.Relationship = jsonapi.ToManyRelationship
			default:
				log.Errorf("Unknown type of relation: %v", relation.GetRelation())
			}

		} else {
			switch relation.GetRelation() {

			case "has_many":
				ref.Type = relation.GetSubject()
				ref.Name = relation.GetSubjectName()
				ref.Relationship = jsonapi.ToManyRelationship
			case "has_one":
				ref.Type = relation.GetSubject()
				ref.Name = relation.GetSubjectName()
				ref.Relationship = jsonapi.ToOneRelationship

			case "belongs_to":
				ref.Type = relation.GetSubject()
				ref.Name = relation.GetSubjectName()
				ref.Relationship = jsonapi.ToManyRelationship
			case "has_many_and_belongs_to_many":
				ref.Type = relation.GetSubject()
				ref.Name = relation.GetSubjectName()
				ref.Relationship = jsonapi.ToManyRelationship
			default:
				log.Errorf("Unknown type of relation: %v", relation.GetRelation())
			}
		}

		references = append(references, ref)
	}

	return references
}

func (m Api2GoModel) GetAttributes() map[string]interface{} {
	attrs := make(map[string]interface{})
	colMap := m.GetColumnMap()

	//log.Infof("Column Map for [%v]: %v", colMap["reference_id"])
	for k, v := range m.data {

		//if colMap[k].IsForeignKey {
		//	continue
		//}

		if colMap[k].ExcludeFromApi {
			continue
		}

		if colMap[k].ColumnType == "password" {
			v = ""
		}

		attrs[k] = v
	}
	return attrs
}

func (m Api2GoModel) GetAllAsAttributes() map[string]interface{} {

	attrs := make(map[string]interface{})
	for k, v := range m.data {
		attrs[k] = v
	}
	attrs["__type"] = m.GetTableName()

	if m.dirty {
		for k, v := range m.oldData {
			_, o := attrs[k]
			if !o {
				attrs[k] = v
			}
		}
	}

	return attrs
}

func (m *Api2GoModel) SetColumns(c []ColumnInfo) {
	m.columns = c

}

func (m Api2GoModel) GetColumns() []ColumnInfo {
	return m.columns
}

func (m Api2GoModel) GetColumnNames() []string {

	colNames := make([]string, 0)
	for _, col := range m.columns {
		colNames = append(colNames, col.ColumnName)
	}

	return colNames
}

func (g Api2GoModel) GetDefaultPermission() int64 {
	//log.Infof("default permission for %v is %v", g.typeName, g.defaultPermission)
	return g.defaultPermission
}

func (g Api2GoModel) GetName() string {
	return g.typeName
}

func (g Api2GoModel) GetTableName() string {
	return g.typeName
}

func (g Api2GoModel) GetColumnOriginalValue(columnName string) interface{} {
	if g.IsDirty() {
		return g.oldData[columnName]
	}
	return g.data[columnName]
}

func (g Api2GoModel) GetID() string {
	if g.IsDirty() {
		id, _ := g.oldData["reference_id"]
		if str, ok := id.(fmt.Stringer); ok {
			return str.String()
		} else {
			return fmt.Sprintf("%s", id)
		}
	}
	id, _ := g.data["reference_id"]
	if str, ok := id.(fmt.Stringer); ok {
		return str.String()
	} else {
		return fmt.Sprintf("%s", id)
	}
}

func (g *Api2GoModel) SetType(typeName string) {
	g.data["__type"] = typeName
}
func (g *Api2GoModel) Set(columnName string, value interface{}) {
	g.data[columnName] = value
}

func isEqual(a, b interface{}) bool {
	if reflect.TypeOf(a) != reflect.TypeOf(b) {
		return false
	}
	if reflect.DeepEqual(a, b) {
		return true
	}
	return false
}

func (g *Api2GoModel) SetAttributes(attrs map[string]interface{}) {
	//log.Infof("set attributes: %v", attrs)
	transformNumbersDict(attrs)
	if g.data == nil {
		g.data = attrs
		return
	}
	for k, v := range attrs {
		strVal, isStr := v.(string)
		if k == "reference_id" && isStr {
			v = uuid.MustParse(strVal)
			attrs[k] = v
			continue
		}

		existingValue, ok := g.data[k]
		if !ok || !isEqual(existingValue, v) {
			if !g.dirty {
				g.dirty = true
				tempMap := make(map[string]interface{})

				for k1, v1 := range g.data {
					tempMap[k1] = v1
				}

				g.oldData = tempMap
			}
			break
		}
	}
	//log.Printf("Set [%v] = [%v]", k, v)
	if g.dirty {
		g.data = attrs
	}
}

type Change struct {
	OldValue interface{}
	NewValue interface{}
}

func (g Api2GoModel) GetAuditModel() Api2GoModel {
	auditTableName := g.GetTableName() + "_audit"

	newData := make(map[string]interface{})

	if g.IsDirty() {
		newData = copyMapWithSkipKeys(g.oldData, []string{"reference_id", "id"})
		//newData["audit_object_id"] = g.oldData["reference_id"]
	} else {
		newData = copyMapWithSkipKeys(g.data, []string{"reference_id", "id"})
		//newData["audit_object_id"] = g.data["reference_id"]
	}

	newData["__type"] = auditTableName

	return NewApi2GoModelWithData(auditTableName, g.columns, g.defaultPermission, nil, newData)

}
func copyMapWithSkipKeys(dataMap map[string]interface{}, skipKeys []string) map[string]interface{} {
	newData := make(map[string]interface{})

	skipMap := make(map[string]bool)
	for _, k := range skipKeys {
		skipMap[k] = true
	}

	for k, v := range dataMap {
		if skipMap[k] {
			continue
		}
		newData[k] = v
	}
	return newData
}

// GetChanges returns a map of changes between the current and old data
// Improved to handle complex types like slices and maps
func (g Api2GoModel) GetChanges() map[string]Change {
	changeMap := make(map[string]Change)

	// Early return if not dirty
	if !g.dirty {
		return changeMap
	}

	// Check what's changed in existing fields
	for key, newVal := range g.data {
		oldVal, exists := g.oldData[key]

		// If key doesn't exist in oldData, it's a new field
		if !exists {
			changeMap[key] = Change{
				OldValue: nil,
				NewValue: newVal,
			}
			continue
		}

		// Use AreValuesEqual for proper deep comparison of all types
		if !AreValuesEqual(oldVal, newVal) {
			changeMap[key] = Change{
				OldValue: oldVal,
				NewValue: newVal,
			}
		}
	}

	return changeMap
}

func (g Api2GoModel) IsDirty() bool {
	return g.dirty
}

func (g Api2GoModel) GetUnmodifiedAttributes() map[string]interface{} {
	if g.dirty {
		return g.oldData
	}
	return g.data
}

func (g *Api2GoModel) SetID(str string) error {
	//log.Infof("set id: %v", str)
	if g.data == nil {
		g.data = make(map[string]interface{})
	}
	g.data["reference_id"] = str
	return nil
}

type HasId interface {
	GetId() interface{}
}

func (g Api2GoModel) GetReferenceId() string {
	return fmt.Sprintf("%v", g.data["reference_id"])
}

func (g *Api2GoModel) BeforeCreate() (err error) {
	u, _ := uuid.NewV7()
	g.data["reference_id"] = u
	return nil
}
