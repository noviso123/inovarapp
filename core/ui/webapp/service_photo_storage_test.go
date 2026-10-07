package webapp

import "testing"

func TestMergeServicePhotosKeepsRemoteAndLocalPhotos(t *testing.T) {
	remote := []appliancePhoto{{Name: "servidor.jpg", Path: "fotos-os/123/servidor.jpg", URL: "https://example.test/signed"}}
	local := []localServicePhoto{{ID: "os-123_1_abcd", ServiceID: "os-123", Name: "local.jpg", DataURL: "data:image/jpeg;base64,AA=="}}

	merged := mergeServicePhotos(remote, local)
	if len(merged) != 2 {
		t.Fatalf("got %d photos, want 2", len(merged))
	}
	if merged[0].Local || merged[0].Path != remote[0].Path {
		t.Fatalf("remote photo changed: %#v", merged[0])
	}
	if !merged[1].Local || merged[1].Path != "local:os-123_1_abcd" || merged[1].URL != local[0].DataURL {
		t.Fatalf("local photo was not mapped to UI: %#v", merged[1])
	}
}

func TestLocalServicePhotoID(t *testing.T) {
	if id, ok := localServicePhotoID("local:photo-id"); !ok || id != "photo-id" {
		t.Fatalf("got (%q, %t), want photo-id", id, ok)
	}
	for _, path := range []string{"", "photo-id", "local:"} {
		if id, ok := localServicePhotoID(path); ok || id != "" {
			t.Errorf("localServicePhotoID(%q) = (%q, %t), want empty/false", path, id, ok)
		}
	}
}
