package paging_test

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/pkg/paging"
)

func TestPage_Normalize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   paging.Page
		want paging.Page
	}{
		{"zero value gets defaults", paging.Page{}, paging.Page{Number: 1, Size: paging.DefaultSize}},
		{"valid page is kept", paging.Page{Number: 3, Size: 50}, paging.Page{Number: 3, Size: 50}},
		{"size above the limit is capped", paging.Page{Number: 2, Size: 500}, paging.Page{Number: 2, Size: paging.MaxSize}},
		{"negative number starts from the first page", paging.Page{Number: -1, Size: 10}, paging.Page{Number: 1, Size: 10}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, tc.in.Normalize())
		})
	}
}

func TestPage_Offset(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0, paging.Page{Number: 1, Size: 20}.Offset())
	require.Equal(t, 40, paging.Page{Number: 3, Size: 20}.Offset())
}

// Дефолт и предел размера живут дважды — в константах и в тегах huma; тест держит их равными.
func TestPage_TagsMatchLimits(t *testing.T) {
	t.Parallel()
	size, ok := reflect.TypeFor[paging.Page]().FieldByName("Size")
	require.True(t, ok)
	require.Equal(t, strconv.Itoa(paging.DefaultSize), size.Tag.Get("default"))
	require.Equal(t, strconv.Itoa(paging.MaxSize), size.Tag.Get("maximum"))
}
