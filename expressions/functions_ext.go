package expressions

// Hand ports of the base parser.FUNCTIONS builders that are not a plain from_arg_list and so
// are skipped by scripts/gen_func_kinds.py (parser.py:138-190 and the FUNCTIONS lambdas).
func init() {
	seq := func(args []Expression, i int) Expression {
		if i < len(args) {
			return args[i]
		}
		return nil
	}
	textCast := func(args []Expression) Expression {
		return New(KindCast, Args{"this": seq(args, 0), "to": DTypeText.IntoExpr(nil)})
	}
	pad := func(isLeft bool) func([]Expression) Expression {
		return func(args []Expression) Expression {
			return New(KindPad, Args{"this": seq(args, 0), "expression": seq(args, 1), "fill_pattern": seq(args, 2), "is_left": isLeft})
		}
	}
	trim := func(isLeft bool) func([]Expression) Expression {
		position := "TRAILING"
		if isLeft {
			position = "LEADING"
		}
		return func(args []Expression) Expression {
			return New(KindTrim, Args{"this": seq(args, 0), "expression": seq(args, 1), "position": position})
		}
	}
	extra := map[string]func([]Expression) Expression{
		"UNNEST": func(args []Expression) Expression {
			var exprs []Expression
			if first := seq(args, 0); first != nil {
				exprs = []Expression{first}
			}
			return New(KindUnnest, Args{"expressions": exprs})
		},
		"VAR_MAP": func(args []Expression) Expression {
			if len(args) == 1 && args[0].IsStar() {
				return New(KindStarMap, Args{"this": args[0]})
			}
			var keys, values []Expression
			for i := 0; i+1 < len(args); i += 2 {
				keys = append(keys, args[i])
				values = append(values, args[i+1])
			}
			return New(KindVarMap, Args{"keys": New(KindArray, Args{"expressions": keys}), "values": New(KindArray, Args{"expressions": values})})
		},
		"DATE_TO_DATE_STR": textCast,
		"TIME_TO_TIME_STR": textCast,
		"TS_OR_DS_TO_DATE_STR": func(args []Expression) Expression {
			return New(KindSubstring, Args{"this": textCast(args), "start": LiteralNumber(1), "length": LiteralNumber(10)})
		},
		"ARRAY_INTERSECT":    func(args []Expression) Expression { return New(KindArrayIntersect, Args{"expressions": args}) },
		"ARRAY_INTERSECTION": func(args []Expression) Expression { return New(KindArrayIntersect, Args{"expressions": args}) },
		"GENERATE_DATE_ARRAY": func(args []Expression) Expression {
			step := seq(args, 2)
			if step == nil {
				step = New(KindInterval, Args{"this": LiteralString("1"), "unit": Var(Args{"this": "DAY"})})
			}
			return New(KindGenerateDateArray, Args{"start": seq(args, 0), "end": seq(args, 1), "step": step})
		},
		"INSTR": func(args []Expression) Expression { return FromArgList(KindStrPosition, args) },
		// parser.py:185-190 build_locate_strposition
		"LOCATE": func(args []Expression) Expression {
			return New(KindStrPosition, Args{"this": seq(args, 1), "substr": seq(args, 0), "position": seq(args, 2)})
		},
		"CHARINDEX": func(args []Expression) Expression {
			return New(KindStrPosition, Args{"this": seq(args, 1), "substr": seq(args, 0), "position": seq(args, 2)})
		},
		// parser.py:411-470: Uuid builders (UUID_IS_STRING_TYPE is False for every ported dialect).
		"UUID":            func([]Expression) Expression { return New(KindUuid, nil) },
		"GEN_RANDOM_UUID": func([]Expression) Expression { return New(KindUuid, nil) },
		"GENERATE_UUID":   func([]Expression) Expression { return New(KindUuid, nil) },
		"UUID_STRING":     func([]Expression) Expression { return New(KindUuid, nil) },
		// parser.py:193-232 build_array_append/build_array_prepend; ARRAY_FUNCS_PROPAGATES_NULLS is
		// False for every ported dialect, so null_propagation stays unset.
		"ARRAY_APPEND": func(args []Expression) Expression {
			return New(KindArrayAppend, Args{"this": seq(args, 0), "expression": seq(args, 1)})
		},
		"ARRAY_PREPEND": func(args []Expression) Expression {
			return New(KindArrayPrepend, Args{"this": seq(args, 0), "expression": seq(args, 1)})
		},
		"LPAD":     pad(true),
		"LEFTPAD":  pad(true),
		"RPAD":     pad(false),
		"RIGHTPAD": pad(false),
		"LTRIM":    trim(true),
		"RTRIM":    trim(false),
	}
	for name, builder := range extra {
		if _, exists := FunctionByName[name]; !exists {
			FunctionByName[name] = builder
		}
	}
}
