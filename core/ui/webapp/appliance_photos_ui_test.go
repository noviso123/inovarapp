package webapp

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func TestAppliancePhotoUploadNoticesMatchReactPartialFailureBehavior(t *testing.T) {
	cases := []struct {
		saved, failed int
		want          string
	}{
		{3, 0, ""},
		{4, 2, "4 salva(s), 2 falharam. Tente novamente."},
		{0, 2, "Não foi possível enviar as fotos. Verifique a conexão e tente de novo."},
	}
	for _, tc := range cases {
		if got := appliancePhotoUploadNotice(tc.saved, tc.failed); got != tc.want {
			t.Errorf("notice(%d,%d)=%q want %q", tc.saved, tc.failed, got, tc.want)
		}
	}
}

func TestAppliancePhotosEmptyCopyMatchesFicha(t *testing.T) {
	p := &serviceCatalogPage{teamPhotoApplianceID: "device-1"}
	markup := app.HTMLString(p.appliancePhotosPanel())
	for _, want := range []string{"Nenhuma foto anexada.", "instalação, serial, local", "ficam salvas na ficha."} {
		if !strings.Contains(markup, want) {
			t.Errorf("empty photo state missing %q: %s", want, markup)
		}
	}
}

func TestAppliancePhotoBatchMatchesReactSixPerSelection(t *testing.T) {
	for _, count := range []int{0, 1, 6, 9} {
		files := make([]appliancePhotoInput, count)
		for index := range files {
			files[index].Name = string(rune('a' + index))
		}
		got := appliancePhotoBatch(files)
		want := count
		if want > 6 {
			want = 6
		}
		if len(got) != want {
			t.Fatalf("batch size for %d selected photos = %d, want %d", count, len(got), want)
		}
		for index := range got {
			if got[index] != files[index] {
				t.Errorf("batch changed selection order at index %d: got %#v, want %#v", index, got[index], files[index])
			}
		}
	}
}
