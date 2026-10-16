package expressions

// hive_nodes.go contains builders for the property, transform, map, and function
// Kinds used by the Hive parser and FUNCTIONS overlay. Their arg_types, traits,
// class names, and variadic metadata live in kinds.go.

// ExternalProperty ports exp.ExternalProperty (.reference/sqlglot-v30.12.0/sqlglot/expressions/properties.py:168-169).
func ExternalProperty(args Args) Expression { return newNode(KindExternalProperty, args) }

// ClusteredByProperty ports exp.ClusteredByProperty (.reference/sqlglot-v30.12.0/sqlglot/expressions/properties.py:230-231).
func ClusteredByProperty(args Args) Expression { return newNode(KindClusteredByProperty, args) }

// LocationProperty ports exp.LocationProperty (.reference/sqlglot-v30.12.0/sqlglot/expressions/properties.py:262-263).
func LocationProperty(args Args) Expression { return newNode(KindLocationProperty, args) }

// StorageHandlerProperty ports exp.StorageHandlerProperty (.reference/sqlglot-v30.12.0/sqlglot/expressions/properties.py:488-489).
func StorageHandlerProperty(args Args) Expression { return newNode(KindStorageHandlerProperty, args) }

// UsingProperty ports exp.UsingProperty (.reference/sqlglot-v30.12.0/sqlglot/expressions/properties.py:492-494).
func UsingProperty(args Args) Expression { return newNode(KindUsingProperty, args) }

// InputOutputFormat ports exp.InputOutputFormat (.reference/sqlglot-v30.12.0/sqlglot/expressions/query.py:878-879).
func InputOutputFormat(args Args) Expression { return newNode(KindInputOutputFormat, args) }

// QueryTransform ports exp.QueryTransform (.reference/sqlglot-v30.12.0/sqlglot/expressions/properties.py:422-431).
func QueryTransform(args Args) Expression { return newNode(KindQueryTransform, args) }

// Transform ports exp.Transform (.reference/sqlglot-v30.12.0/sqlglot/expressions/array.py:214-215).
func Transform(args Args) Expression { return newNode(KindTransform, args) }

// ToBase64 ports exp.ToBase64 (.reference/sqlglot-v30.12.0/sqlglot/expressions/string.py:325-326).
func ToBase64(args Args) Expression { return newNode(KindToBase64, args) }

// FromBase64 ports exp.FromBase64 (.reference/sqlglot-v30.12.0/sqlglot/expressions/string.py:301-302).
func FromBase64(args Args) Expression { return newNode(KindFromBase64, args) }

// TsOrDsAdd ports exp.TsOrDsAdd (.reference/sqlglot-v30.12.0/sqlglot/expressions/temporal.py:145-147).
func TsOrDsAdd(args Args) Expression { return newNode(KindTsOrDsAdd, args) }

// TsOrDsToDate ports exp.TsOrDsToDate (.reference/sqlglot-v30.12.0/sqlglot/expressions/temporal.py:492-493).
func TsOrDsToDate(args Args) Expression { return newNode(KindTsOrDsToDate, args) }

// First ports exp.First (.reference/sqlglot-v30.12.0/sqlglot/expressions/aggregate.py:125-126).
func First(args Args) Expression { return newNode(KindFirst, args) }

// FirstValue ports exp.FirstValue (.reference/sqlglot-v30.12.0/sqlglot/expressions/aggregate.py:129-130).
func FirstValue(args Args) Expression { return newNode(KindFirstValue, args) }

// Last ports exp.Last (.reference/sqlglot-v30.12.0/sqlglot/expressions/aggregate.py:155-156).
func Last(args Args) Expression { return newNode(KindLast, args) }

// LastValue ports exp.LastValue (.reference/sqlglot-v30.12.0/sqlglot/expressions/aggregate.py:159-160).
func LastValue(args Args) Expression { return newNode(KindLastValue, args) }

// RegexpExtract ports exp.RegexpExtract (.reference/sqlglot-v30.12.0/sqlglot/expressions/string.py:421-430).
func RegexpExtract(args Args) Expression { return newNode(KindRegexpExtract, args) }

// RegexpExtractAll ports exp.RegexpExtractAll (.reference/sqlglot-v30.12.0/sqlglot/expressions/string.py:433-441).
func RegexpExtractAll(args Args) Expression { return newNode(KindRegexpExtractAll, args) }

// TimestampTrunc ports exp.TimestampTrunc (.reference/sqlglot-v30.12.0/sqlglot/expressions/temporal.py:190-191).
func TimestampTrunc(args Args) Expression { return newNode(KindTimestampTrunc, args) }

// UnixToStr ports exp.UnixToStr (.reference/sqlglot-v30.12.0/sqlglot/expressions/temporal.py:528-529).
func UnixToStr(args Args) Expression { return newNode(KindUnixToStr, args) }

// TimeToStr ports exp.TimeToStr (.reference/sqlglot-v30.12.0/sqlglot/expressions/temporal.py:476-477).
func TimeToStr(args Args) Expression { return newNode(KindTimeToStr, args) }

// StarMap ports exp.StarMap (.reference/sqlglot-v30.12.0/sqlglot/expressions/array.py:331-332).
func StarMap(args Args) Expression { return newNode(KindStarMap, args) }

// VarMap ports exp.VarMap (.reference/sqlglot-v30.12.0/sqlglot/expressions/array.py:339-341).
func VarMap(args Args) Expression { return newNode(KindVarMap, args) }

// TimeseriesKey ports exp.TimeseriesKey (constraints.py:224-225).
func TimeseriesKey(args Args) Expression { return newNode(KindTimeseriesKey, args) }

// ClusterProperty ports exp.ClusterProperty (properties.py:90-91).
func ClusterProperty(args Args) Expression { return newNode(KindClusterProperty, args) }

// Stream ports exp.Stream (query.py:2085-2086).
func Stream(args Args) Expression { return newNode(KindStream, args) }

// EnviromentProperty ports exp.EnviromentProperty (properties.py:234-235).
func EnviromentProperty(args Args) Expression { return newNode(KindEnviromentProperty, args) }

// HandlerProperty ports exp.HandlerProperty (properties.py:160-161).
func HandlerProperty(args Args) Expression { return newNode(KindHandlerProperty, args) }

// ParameterStyleProperty ports exp.ParameterStyleProperty (properties.py:164-165).
func ParameterStyleProperty(args Args) Expression { return newNode(KindParameterStyleProperty, args) }

// StreamingTableProperty ports exp.StreamingTableProperty (properties.py:393-394).
func StreamingTableProperty(args Args) Expression { return newNode(KindStreamingTableProperty, args) }

// Clone ports exp.Clone (ddl.py:117-118).
func Clone(args Args) Expression { return newNode(KindClone, args) }
