package string

import (
	"errors"
	"fmt"

	"github.com/goccy/googlesqlite/internal/functions/helper"
	"github.com/goccy/googlesqlite/internal/value"
)

func LIKE(a, b value.Value) (value.Value, error) {
	// A STRING pattern matches character by character, a BYTES pattern
	// byte by byte, so the match units differ per operand type.
	_, patternIsBytes := b.(value.BytesValue)
	if _, ok := a.(value.BytesValue); ok {
		if !patternIsBytes {
			return nil, fmt.Errorf("LIKE: value and pattern must be the same type")
		}
		v, err := a.ToBytes()
		if err != nil {
			return nil, err
		}
		pattern, err := b.ToBytes()
		if err != nil {
			return nil, err
		}
		return likeResult(v, pattern)
	}
	if patternIsBytes {
		return nil, fmt.Errorf("LIKE: value and pattern must be the same type")
	}
	v, err := a.ToString()
	if err != nil {
		return nil, err
	}
	pattern, err := b.ToString()
	if err != nil {
		return nil, err
	}
	return likeResult([]rune(v), []rune(pattern))
}

func likeResult[T byte | rune](subject, pattern []T) (value.Value, error) {
	units, err := compileLikePattern(pattern)
	if err != nil {
		return nil, err
	}
	return value.BoolValue(matchLikeUnits(subject, units)), nil
}

type likeUnitKind int

const (
	likeLiteral likeUnitKind = iota
	likeAnyOne
	likeAnySequence
)

type likeUnit[T byte | rune] struct {
	kind likeUnitKind
	lit  T
}

// compileLikePattern splits a LIKE pattern into match units: '%' matches
// any number of units, '_' exactly one, a backslash escapes the unit
// that follows it, and everything else matches itself.
func compileLikePattern[T byte | rune](pattern []T) ([]likeUnit[T], error) {
	units := make([]likeUnit[T], 0, len(pattern))
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '\\':
			if i+1 == len(pattern) {
				return nil, errors.New("LIKE pattern ends with a backslash")
			}
			i++
			units = append(units, likeUnit[T]{kind: likeLiteral, lit: pattern[i]})
		case '_':
			units = append(units, likeUnit[T]{kind: likeAnyOne})
		case '%':
			units = append(units, likeUnit[T]{kind: likeAnySequence})
		default:
			units = append(units, likeUnit[T]{kind: likeLiteral, lit: c})
		}
	}
	return units, nil
}

// matchLikeUnits consumes the subject greedily, remembering the most
// recent '%' so that a later mismatch resumes it one unit further along
// instead of failing the whole pattern.
func matchLikeUnits[T byte | rune](subject []T, units []likeUnit[T]) bool {
	var subjectIdx, unitIdx int
	resumeUnit, resumeSubject := -1, 0
	for subjectIdx < len(subject) {
		if unitIdx < len(units) {
			switch u := units[unitIdx]; u.kind {
			case likeAnySequence:
				resumeUnit, resumeSubject = unitIdx, subjectIdx
				unitIdx++
				continue
			case likeAnyOne:
				unitIdx++
				subjectIdx++
				continue
			case likeLiteral:
				if u.lit == subject[subjectIdx] {
					unitIdx++
					subjectIdx++
					continue
				}
			}
		}
		if resumeUnit < 0 {
			return false
		}
		resumeSubject++
		unitIdx, subjectIdx = resumeUnit+1, resumeSubject
	}
	for unitIdx < len(units) && units[unitIdx].kind == likeAnySequence {
		unitIdx++
	}
	return unitIdx == len(units)
}

// BindLike: per GoogleSQL three-valued logic, LIKE with a NULL operand
// returns NULL, not FALSE — Scalar2 propagates that.
var BindLike = helper.Scalar2(LIKE)
