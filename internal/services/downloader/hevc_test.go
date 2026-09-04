package downloader

import (
	"reflect"
	"testing"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

func TestIsHEVCSource(t *testing.T) {
	hevc := []string{"h265", "H265", "hevc", "HEVC", "hvc1", "hev1"}
	for _, c := range hevc {
		if !isHEVCSource(c) {
			t.Errorf("isHEVCSource(%q) = false, want true", c)
		}
	}
	// Anything not recognisably HEVC must convert: an unknown codec that slipped
	// through as "already fine" would leave the file unplayable on the very
	// device the option exists for.
	other := []string{"h264", "avc1", "AVC", "", "vp9", "av1", "unknown"}
	for _, c := range other {
		if isHEVCSource(c) {
			t.Errorf("isHEVCSource(%q) = true, want false", c)
		}
	}
}

// The branch that matters: a source already in HEVC must not be handed to an
// encoder. Re-encoding it would cost hours and lose quality for nothing.
func TestEffectiveArgsSkipsEncoderWhenSourceIsHEVC(t *testing.T) {
	manual := []string{"-metadata", "comment=x"}
	job := domain.Job{Media: domain.ResolvedMedia{
		Source: domain.MediaSource{Codec: "h265"},
	}}

	d := &Downloader{transcodeHEVC: true, extraArgs: manual}
	if got := d.effectiveArgs(job); !reflect.DeepEqual(got, manual) {
		t.Errorf("HEVC source: effectiveArgs = %v, want just the manual args %v", got, manual)
	}

	// Not asked for at all → likewise untouched, whatever the source is.
	d = &Downloader{transcodeHEVC: false, extraArgs: manual}
	job.Media.Source.Codec = "h264"
	if got := d.effectiveArgs(job); !reflect.DeepEqual(got, manual) {
		t.Errorf("flag off: effectiveArgs = %v, want %v", got, manual)
	}
}
