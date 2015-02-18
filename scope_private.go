package gorm

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

func (scope *Scope) primaryCondition(value interface{}) string {
	return fmt.Sprintf("(%v = %v)", scope.Quote(scope.PrimaryKey()), value)
}

func (scope *Scope) buildWhereCondition(clause map[string]interface{}) (str string) {
	switch value := clause["query"].(type) {
	case string:
		// if string is number
		if regexp.MustCompile("^\\s*\\d+\\s*$").MatchString(value) {
			id, _ := strconv.Atoi(value)
			return scope.primaryCondition(scope.AddToVars(id))
		} else if value != "" {
			str = fmt.Sprintf("(%v)", value)
		}
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, sql.NullInt64:
		return scope.primaryCondition(scope.AddToVars(value))
	case []int, []int8, []int16, []int32, []int64, []uint, []uint8, []uint16, []uint32, []uint64, []string, []interface{}:
		str = fmt.Sprintf("(%v in (?))", scope.Quote(scope.PrimaryKey()))
		clause["args"] = []interface{}{value}
	case map[string]interface{}:
		var sqls []string
		for key, value := range value {
			sqls = append(sqls, fmt.Sprintf("(%v = %v)", scope.Quote(key), scope.AddToVars(value)))
		}
		return strings.Join(sqls, " AND ")
	case interface{}:
		var sqls []string
		for _, field := range scope.New(value).Fields() {
			if !field.IsBlank {
				sqls = append(sqls, fmt.Sprintf("(%v = %v)", scope.Quote(field.DBName), scope.AddToVars(field.Field.Interface())))
			}
		}
		return strings.Join(sqls, " AND ")
	}

	args := clause["args"].([]interface{})
	for _, arg := range args {
		switch reflect.TypeOf(arg).Kind() {
		case reflect.Slice: // For where("id in (?)", []int64{1,2})
			values := reflect.ValueOf(arg)
			var tempMarks []string
			for i := 0; i < values.Len(); i++ {
				tempMarks = append(tempMarks, scope.AddToVars(values.Index(i).Interface()))
			}
			str = strings.Replace(str, "?", strings.Join(tempMarks, ","), 1)
		default:
			if valuer, ok := interface{}(arg).(driver.Valuer); ok {
				arg, _ = valuer.Value()
			}

			str = strings.Replace(str, "?", scope.AddToVars(arg), 1)
		}
	}
	return
}

func (scope *Scope) buildNotCondition(clause map[string]interface{}) (str string) {
	var notEqualSql string
	var primaryKey = scope.PrimaryKey()

	switch value := clause["query"].(type) {
	case string:
		// is number
		if regexp.MustCompile("^\\s*\\d+\\s*$").MatchString(value) {
			id, _ := strconv.Atoi(value)
			return fmt.Sprintf("(%v <> %v)", scope.Quote(primaryKey), id)
		} else if regexp.MustCompile("(?i) (=|<>|>|<|LIKE|IS) ").MatchString(value) {
			str = fmt.Sprintf(" NOT (%v) ", value)
			notEqualSql = fmt.Sprintf("NOT (%v)", value)
		} else {
			str = fmt.Sprintf("(%v NOT IN (?))", scope.Quote(value))
			notEqualSql = fmt.Sprintf("(%v <> ?)", scope.Quote(value))
		}
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, sql.NullInt64:
		return fmt.Sprintf("(%v <> %v)", scope.Quote(primaryKey), value)
	case []int, []int8, []int16, []int32, []int64, []uint, []uint8, []uint16, []uint32, []uint64, []string:
		if reflect.ValueOf(value).Len() > 0 {
			str = fmt.Sprintf("(%v NOT IN (?))", scope.Quote(primaryKey))
			clause["args"] = []interface{}{value}
		}
		return ""
	case map[string]interface{}:
		var sqls []string
		for key, value := range value {
			sqls = append(sqls, fmt.Sprintf("(%v <> %v)", scope.Quote(key), scope.AddToVars(value)))
		}
		return strings.Join(sqls, " AND ")
	case interface{}:
		var sqls []string
		for _, field := range scope.New(value).Fields() {
			if !field.IsBlank {
				sqls = append(sqls, fmt.Sprintf("(%v <> %v)", scope.Quote(field.DBName), scope.AddToVars(field.Field.Interface())))
			}
		}
		return strings.Join(sqls, " AND ")
	}

	args := clause["args"].([]interface{})
	for _, arg := range args {
		switch reflect.TypeOf(arg).Kind() {
		case reflect.Slice: // For where("id in (?)", []int64{1,2})
			values := reflect.ValueOf(arg)
			var tempMarks []string
			for i := 0; i < values.Len(); i++ {
				tempMarks = append(tempMarks, scope.AddToVars(values.Index(i).Interface()))
			}
			str = strings.Replace(str, "?", strings.Join(tempMarks, ","), 1)
		default:
			if scanner, ok := interface{}(arg).(driver.Valuer); ok {
				arg, _ = scanner.Value()
			}
			str = strings.Replace(notEqualSql, "?", scope.AddToVars(arg), 1)
		}
	}
	return
}

func (scope *Scope) buildSelectQuery(clause map[string]interface{}) (str string) {
	switch value := clause["query"].(type) {
	case string:
		str = value
	case []string:
		str = strings.Join(value, ", ")
	}

	args := clause["args"].([]interface{})
	for _, arg := range args {
		switch reflect.TypeOf(arg).Kind() {
		case reflect.Slice:
			values := reflect.ValueOf(arg)
			var tempMarks []string
			for i := 0; i < values.Len(); i++ {
				tempMarks = append(tempMarks, scope.AddToVars(values.Index(i).Interface()))
			}
			str = strings.Replace(str, "?", strings.Join(tempMarks, ","), 1)
		default:
			if valuer, ok := interface{}(arg).(driver.Valuer); ok {
				arg, _ = valuer.Value()
			}
			str = strings.Replace(str, "?", scope.Dialect().Quote(fmt.Sprintf("%v", arg)), 1)
		}
	}
	return
}

func (scope *Scope) whereSql() (sql string) {
	var primaryConditions, andConditions, orConditions []string

	if !scope.Search.Unscope && scope.Fields()["deleted_at"] != nil {
		sql := fmt.Sprintf("(%v.deleted_at IS NULL OR %v.deleted_at <= '0001-01-02')", scope.QuotedTableName(), scope.QuotedTableName())
		primaryConditions = append(primaryConditions, sql)
	}

	if !scope.PrimaryKeyZero() {
		primaryConditions = append(primaryConditions, scope.primaryCondition(scope.AddToVars(scope.PrimaryKeyValue())))
	}

	for _, clause := range scope.Search.WhereConditions {
		if sql := scope.buildWhereCondition(clause); sql != "" {
			andConditions = append(andConditions, sql)
		}
	}

	for _, clause := range scope.Search.OrConditions {
		if sql := scope.buildWhereCondition(clause); sql != "" {
			orConditions = append(orConditions, sql)
		}
	}

	for _, clause := range scope.Search.NotConditions {
		if sql := scope.buildNotCondition(clause); sql != "" {
			andConditions = append(andConditions, sql)
		}
	}

	orSql := strings.Join(orConditions, " OR ")
	combinedSql := strings.Join(andConditions, " AND ")
	if len(combinedSql) > 0 {
		if len(orSql) > 0 {
			combinedSql = combinedSql + " OR " + orSql
		}
	} else {
		combinedSql = orSql
	}

	if len(primaryConditions) > 0 {
		sql = "WHERE " + strings.Join(primaryConditions, " AND ")
		if len(combinedSql) > 0 {
			sql = sql + " AND (" + combinedSql + ")"
		}
	} else if len(combinedSql) > 0 {
		sql = "WHERE " + combinedSql
	}
	return
}

func (scope *Scope) selectSql() string {
	if len(scope.Search.Selects) == 0 {
		return "*"
	}

	var selectQueries []string

	for _, clause := range scope.Search.Selects {
		selectQueries = append(selectQueries, scope.buildSelectQuery(clause))
	}

	return strings.Join(selectQueries, ", ")
}

func (scope *Scope) orderSql() string {
	if len(scope.Search.Orders) == 0 {
		return ""
	}
	return " ORDER BY " + strings.Join(scope.Search.Orders, ",")
}

func (scope *Scope) limitSql() string {
	if !scope.Dialect().HasTop() {
		if len(scope.Search.Limit) == 0 {
			return ""
		}
		return " LIMIT " + scope.Search.Limit
	}

	return ""
}

func (scope *Scope) topSql() string {
	if scope.Dialect().HasTop() && len(scope.Search.Offset) == 0 {
		if len(scope.Search.Limit) == 0 {
			return ""
		}
		return " TOP(" + scope.Search.Limit + ")"
	}

	return ""
}

func (scope *Scope) offsetSql() string {
	if len(scope.Search.Offset) == 0 {
		return ""
	}

	if scope.Dialect().HasTop() {
		sql := " OFFSET " + scope.Search.Offset + " ROW "
		if len(scope.Search.Limit) > 0 {
			sql += "FETCH NEXT " + scope.Search.Limit + " ROWS ONLY"
		}
		return sql
	}
	return " OFFSET " + scope.Search.Offset
}

func (scope *Scope) groupSql() string {
	if len(scope.Search.Group) == 0 {
		return ""
	}
	return " GROUP BY " + scope.Search.Group
}

func (scope *Scope) havingSql() string {
	if scope.Search.HavingCondition == nil {
		return ""
	}
	return " HAVING " + scope.buildWhereCondition(scope.Search.HavingCondition)
}

func (scope *Scope) joinsSql() string {
	return scope.Search.Joins + " "
}

func (scope *Scope) prepareQuerySql() {
	if scope.Search.Raw {
		scope.Raw(strings.TrimSuffix(strings.TrimPrefix(scope.CombinedConditionSql(), " WHERE ("), ")"))
	} else {
		scope.Raw(fmt.Sprintf("SELECT %v %v FROM %v %v", scope.topSql(), scope.selectSql(), scope.QuotedTableName(), scope.CombinedConditionSql()))
	}
	return
}

func (scope *Scope) inlineCondition(values ...interface{}) *Scope {
	if len(values) > 0 {
		scope.Search = scope.Search.clone().where(values[0], values[1:]...)
	}
	return scope
}

func (scope *Scope) callCallbacks(funcs []*func(s *Scope)) *Scope {
	for _, f := range funcs {
		(*f)(scope)
		if scope.skipLeft {
			break
		}
	}
	return scope
}

func (scope *Scope) updatedAttrsWithValues(values map[string]interface{}, ignoreProtectedAttrs bool) (results map[string]interface{}, hasUpdate bool) {
	if !scope.IndirectValue().CanAddr() {
		return values, true
	}

	fields := scope.Fields()
	for key, value := range values {
		if field, ok := fields[ToDBName(key)]; ok && field.Field.IsValid() {
			if !reflect.DeepEqual(field.Field, reflect.ValueOf(value)) {
				if !equalAsString(field.Field.Interface(), value) {
					hasUpdate = true
					field.Set(value)
				}
			}
		}
	}
	return
}

func (scope *Scope) row() *sql.Row {
	defer scope.Trace(NowFunc())
	scope.prepareQuerySql()
	return scope.DB().QueryRow(scope.Sql, scope.SqlVars...)
}

func (scope *Scope) rows() (*sql.Rows, error) {
	defer scope.Trace(NowFunc())
	scope.prepareQuerySql()
	return scope.DB().Query(scope.Sql, scope.SqlVars...)
}

func (scope *Scope) initialize() *Scope {
	for _, clause := range scope.Search.WhereConditions {
		scope.updatedAttrsWithValues(convertInterfaceToMap(clause["query"]), false)
	}
	scope.updatedAttrsWithValues(convertInterfaceToMap(scope.Search.InitAttrs), false)
	scope.updatedAttrsWithValues(convertInterfaceToMap(scope.Search.AssignAttrs), false)
	return scope
}

func (scope *Scope) pluck(column string, value interface{}) *Scope {
	dest := reflect.Indirect(reflect.ValueOf(value))
	scope.Search = scope.Search.clone().selects(column)
	if dest.Kind() != reflect.Slice {
		scope.Err(errors.New("results should be a slice"))
		return scope
	}

	rows, err := scope.rows()
	if scope.Err(err) == nil {
		defer rows.Close()
		for rows.Next() {
			elem := reflect.New(dest.Type().Elem()).Interface()
			scope.Err(rows.Scan(elem))
			dest.Set(reflect.Append(dest, reflect.ValueOf(elem).Elem()))
		}
	}
	return scope
}

func (scope *Scope) count(value interface{}) *Scope {
	scope.Search = scope.Search.clone().selects("count(*)")
	scope.Err(scope.row().Scan(value))
	return scope
}

func (scope *Scope) typeName() string {
	value := scope.IndirectValue()
	if value.Kind() == reflect.Slice {
		return value.Type().Elem().Name()
	}

	return value.Type().Name()
}

func (scope *Scope) related(value interface{}, foreignKeys ...string) *Scope {
	toScope := scope.db.NewScope(value)
	fromFields := scope.Fields()
	toFields := toScope.Fields()
	for _, foreignKey := range append(foreignKeys, toScope.typeName()+"Id", scope.typeName()+"Id") {
		fromField := fromFields[ToDBName(foreignKey)]
		toField := toFields[ToDBName(foreignKey)]

		if fromField != nil {
			if relationship := fromField.Relationship; relationship != nil {
				if relationship.Kind == "many_to_many" {
					joinSql := fmt.Sprintf(
						"INNER JOIN %v ON %v.%v = %v.%v",
						scope.Quote(relationship.JoinTable),
						scope.Quote(relationship.JoinTable),
						scope.Quote(relationship.AssociationForeignDBName),
						toScope.QuotedTableName(),
						scope.Quote(toScope.PrimaryKey()))
					whereSql := fmt.Sprintf("%v.%v = ?", scope.Quote(relationship.JoinTable), scope.Quote(relationship.ForeignDBName))
					scope.Err(toScope.db.Joins(joinSql).Where(whereSql, scope.PrimaryKeyValue()).Find(value).Error)
				} else if relationship.Kind == "belongs_to" {
					sql := fmt.Sprintf("%v = ?", scope.Quote(toScope.PrimaryKey()))
					scope.Err(toScope.db.Where(sql, fromField.Field.Interface()).Find(value).Error)
				} else if relationship.Kind == "has_many" || relationship.Kind == "has_one" {
					sql := fmt.Sprintf("%v = ?", scope.Quote(relationship.ForeignDBName))
					query := toScope.db.Where(sql, scope.PrimaryKeyValue())
					if relationship.ForeignType != "" && toScope.HasColumn(relationship.ForeignType) {
						query = query.Where(fmt.Sprintf("%v = ?", scope.Quote(ToDBName(relationship.ForeignType))), scope.TableName())
					}
					scope.Err(query.Find(value).Error)
				}
			} else {
				sql := fmt.Sprintf("%v = ?", scope.Quote(toScope.PrimaryKey()))
				scope.Err(toScope.db.Where(sql, fromField.Field.Interface()).Find(value).Error)
			}
			return scope
		} else if toField != nil {
			sql := fmt.Sprintf("%v = ?", scope.Quote(toField.DBName))
			scope.Err(toScope.db.Where(sql, scope.PrimaryKeyValue()).Find(value).Error)
			return scope
		}
	}

	scope.Err(fmt.Errorf("invalid association %v", foreignKeys))
	return scope
}

func (scope *Scope) createJoinTable(field *StructField) {
	if field.Relationship != nil && field.Relationship.JoinTable != "" {
		if !scope.Dialect().HasTable(scope, field.Relationship.JoinTable) {
			newScope := scope.db.NewScope("")
			primaryKeySqlType := scope.Dialect().SqlTag(scope.PrimaryKeyField().Field, 255)
			newScope.Raw(fmt.Sprintf("CREATE TABLE %v (%v)",
				field.Relationship.JoinTable,
				strings.Join([]string{
					scope.Quote(field.Relationship.ForeignDBName) + " " + primaryKeySqlType,
					scope.Quote(field.Relationship.AssociationForeignDBName) + " " + primaryKeySqlType}, ",")),
			).Exec()
			scope.Err(newScope.db.Error)
		}
	}
}

func (scope *Scope) createTable() *Scope {
	var sqls []string
	for _, structField := range scope.GetStructFields() {
		if structField.IsNormal {
			sqls = append(sqls, scope.Quote(structField.DBName)+" "+structField.SqlTag)
		}
		scope.createJoinTable(structField)
	}
	scope.Raw(fmt.Sprintf("CREATE TABLE %v (%v)", scope.QuotedTableName(), strings.Join(sqls, ","))).Exec()
	return scope
}

func (scope *Scope) dropTable() *Scope {
	scope.Raw(fmt.Sprintf("DROP TABLE %v", scope.QuotedTableName())).Exec()
	return scope
}

func (scope *Scope) dropTableIfExists() *Scope {
	scope.Raw(fmt.Sprintf("DROP TABLE IF EXISTS %v", scope.QuotedTableName())).Exec()
	return scope
}

func (scope *Scope) modifyColumn(column string, typ string) {
	scope.Raw(fmt.Sprintf("ALTER TABLE %v MODIFY %v %v", scope.QuotedTableName(), scope.Quote(column), typ)).Exec()
}

func (scope *Scope) dropColumn(column string) {
	scope.Raw(fmt.Sprintf("ALTER TABLE %v DROP COLUMN %v", scope.QuotedTableName(), scope.Quote(column))).Exec()
}

func (scope *Scope) addIndex(unique bool, indexName string, column ...string) {
	var columns []string
	for _, name := range column {
		columns = append(columns, scope.Quote(name))
	}

	sqlCreate := "CREATE INDEX"
	if unique {
		sqlCreate = "CREATE UNIQUE INDEX"
	}

	scope.Raw(fmt.Sprintf("%s %v ON %v(%v);", sqlCreate, indexName, scope.QuotedTableName(), strings.Join(columns, ", "))).Exec()
}

func (scope *Scope) addForeignKey(field string, dest string, onDelete string, onUpdate string) {
	var table = scope.TableName()
	var keyName = fmt.Sprintf("%s_%s_foreign", table, field)
	var query = `ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s ON DELETE %s ON UPDATE %s;`
	scope.Raw(fmt.Sprintf(query, table, keyName, field, dest, onDelete, onUpdate)).Exec()
}

func (scope *Scope) removeIndex(indexName string) {
	scope.Dialect().RemoveIndex(scope, indexName)
}

func (scope *Scope) autoMigrate() *Scope {
	tableName := scope.TableName()
	quotedTableName := scope.QuotedTableName()

	if !scope.Dialect().HasTable(scope, tableName) {
		scope.createTable()
	} else {
		for _, field := range scope.GetStructFields() {
			if !scope.Dialect().HasColumn(scope, tableName, field.DBName) {
				if field.IsNormal {
					scope.Raw(fmt.Sprintf("ALTER TABLE %v ADD %v %v;", quotedTableName, field.DBName, field.SqlTag)).Exec()
				}
			}
			scope.createJoinTable(field)
		}
	}
	return scope
}
