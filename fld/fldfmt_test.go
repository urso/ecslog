package fld

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFlags(t *testing.T) {
	testCases := map[string]struct {
		s         string
		startPos  int
		expPos    int
		provState *state
		expState  *state
		expMatch  bool
	}{
		"sharp (#)": {
			s:        "#",
			expPos:   1,
			expState: &state{flags: flags{sharp: true}},
			expMatch: true,
		},
		"plus (+)": {
			s:        "+",
			expPos:   1,
			expState: &state{flags: flags{plus: true}},
			expMatch: true,
		},
		"minus (-)": {
			s:        "-",
			expPos:   1,
			expState: &state{flags: flags{minus: true, zero: false}},
			expMatch: true,
		},
		"zero (0) when minutes is set to true": {
			s:         "0",
			expPos:    1,
			provState: &state{flags: flags{minus: true}},
			expState:  &state{flags: flags{minus: true, zero: false}},
			expMatch:  true,
		},
		"zero (0) when minutes is set to false": {
			s:         "0",
			expPos:    1,
			provState: &state{flags: flags{minus: false}},
			expState:  &state{flags: flags{minus: true, zero: true}},
			expMatch:  true,
		},
		"space (' ')": {
			s:        " ",
			expPos:   1,
			expState: &state{flags: flags{space: true}},
			expMatch: true,
		},
		"no match": {
			s:        "A",
			expPos:   0,
			expState: &state{flags: flags{}},
			expMatch: false,
		},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			if test.provState == nil {
				test.provState = &state{flags: flags{}}
			}

			pos, match := parseFlag(test.provState, test.s, test.startPos)
			require.Equal(t, test.expPos, pos)
			require.Equal(t, test.expMatch, match)
		})
	}
}
