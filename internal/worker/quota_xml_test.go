package worker

import (
	"strconv"
	"strings"
	"testing"
)

func TestCalculateAvailableBytes(t *testing.T) {
	// Limited quota
	if got := CalculateAvailableBytes(1000, 200); got != 800 {
		t.Errorf("expected 800, got %d", got)
	}
	// Quota exceeded
	if got := CalculateAvailableBytes(500, 600); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
	// Quota exactly met
	if got := CalculateAvailableBytes(500, 500); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
	// Unlimited quota, 0 used
	if got := CalculateAvailableBytes(0, 0); got != UnlimitedQuotaCeiling {
		t.Errorf("expected UnlimitedQuotaCeiling, got %d", got)
	}
	// Unlimited quota, some used
	if got := CalculateAvailableBytes(0, 500); got != UnlimitedQuotaCeiling-500 {
		t.Errorf("expected %d, got %d", UnlimitedQuotaCeiling-500, got)
	}
	// Negative quota limit treated as unlimited
	if got := CalculateAvailableBytes(-1, 200); got != UnlimitedQuotaCeiling-200 {
		t.Errorf("expected %d, got %d", UnlimitedQuotaCeiling-200, got)
	}
}

func TestParsePropfindRequest(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantAvailable bool
		wantUsed      bool
	}{
		{
			name:          "empty body treated as allprop",
			body:          "",
			wantAvailable: true,
			wantUsed:      true,
		},
		{
			name:          "whitespace only body treated as allprop",
			body:          "   \n  \t ",
			wantAvailable: true,
			wantUsed:      true,
		},
		{
			name: "explicit allprop",
			body: `<?xml version="1.0" encoding="utf-8" ?>
<D:propfind xmlns:D="DAV:">
  <D:allprop/>
</D:propfind>`,
			wantAvailable: true,
			wantUsed:      true,
		},
		{
			name: "propname request",
			body: `<?xml version="1.0" encoding="utf-8" ?>
<D:propfind xmlns:D="DAV:">
  <D:propname/>
</D:propfind>`,
			wantAvailable: false,
			wantUsed:      false,
		},
		{
			name: "both quota props requested",
			body: `<?xml version="1.0" encoding="utf-8" ?>
<D:propfind xmlns:D="DAV:">
  <D:prop>
    <D:quota-available-bytes/>
    <D:quota-used-bytes/>
  </D:prop>
</D:propfind>`,
			wantAvailable: true,
			wantUsed:      true,
		},
		{
			name: "only quota-available-bytes",
			body: `<?xml version="1.0" encoding="utf-8" ?>
<D:propfind xmlns:D="DAV:">
  <D:prop>
    <D:quota-available-bytes/>
  </D:prop>
</D:propfind>`,
			wantAvailable: true,
			wantUsed:      false,
		},
		{
			name: "only quota-used-bytes",
			body: `<?xml version="1.0" encoding="utf-8" ?>
<D:propfind xmlns:D="DAV:">
  <D:prop>
    <D:quota-used-bytes/>
  </D:prop>
</D:propfind>`,
			wantAvailable: false,
			wantUsed:      true,
		},
		{
			name: "other props only",
			body: `<?xml version="1.0" encoding="utf-8" ?>
<D:propfind xmlns:D="DAV:">
  <D:prop>
    <D:displayname/>
    <D:getcontentlength/>
  </D:prop>
</D:propfind>`,
			wantAvailable: false,
			wantUsed:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotAvail, gotUsed := ParsePropfindRequest([]byte(tc.body))
			if gotAvail != tc.wantAvailable || gotUsed != tc.wantUsed {
				t.Errorf("ParsePropfindRequest() = (%v, %v), want (%v, %v)", gotAvail, gotUsed, tc.wantAvailable, tc.wantUsed)
			}
		})
	}
}

func TestInjectQuotaProperties(t *testing.T) {
	// Case 1: allprop with limited quota
	inputAllprop := `<?xml version="1.0" encoding="UTF-8"?><D:multistatus xmlns:D="DAV:"><D:response><D:href>/webdav/</D:href><D:propstat><D:prop><D:displayname/></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response></D:multistatus>`
	out, err := InjectQuotaProperties([]byte(inputAllprop), 1000, 200, true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "<D:quota-available-bytes>800</D:quota-available-bytes>") {
		t.Errorf("expected quota-available-bytes 800, got: %s", outStr)
	}
	if !strings.Contains(outStr, "<D:quota-used-bytes>200</D:quota-used-bytes>") {
		t.Errorf("expected quota-used-bytes 200, got: %s", outStr)
	}

	// Case 2: specific quota props only (404 propstat emptied and removed, 200 created)
	inputSpecific := `<?xml version="1.0" encoding="UTF-8"?><D:multistatus xmlns:D="DAV:"><D:response><D:href>/webdav/</D:href><D:propstat><D:prop><D:quota-available-bytes/><D:quota-used-bytes/></D:prop><D:status>HTTP/1.1 404 Not Found</D:status></D:propstat></D:response></D:multistatus>`
	out, err = InjectQuotaProperties([]byte(inputSpecific), 500, 100, true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outStr = string(out)
	if strings.Contains(outStr, "404 Not Found") {
		t.Errorf("404 propstat should have been removed, got: %s", outStr)
	}
	if !strings.Contains(outStr, "<D:quota-available-bytes>400</D:quota-available-bytes>") {
		t.Errorf("expected quota-available-bytes 400, got: %s", outStr)
	}
	if !strings.Contains(outStr, "<D:quota-used-bytes>100</D:quota-used-bytes>") {
		t.Errorf("expected quota-used-bytes 100, got: %s", outStr)
	}

	// Case 3: unlimited quota
	out, err = InjectQuotaProperties([]byte(inputSpecific), 0, 100, true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outStr = string(out)
	expectedAvail := strconv.FormatInt(UnlimitedQuotaCeiling-100, 10)
	if !strings.Contains(outStr, "<D:quota-available-bytes>"+expectedAvail+"</D:quota-available-bytes>") {
		t.Errorf("expected unlimited quota-available-bytes %s, got: %s", expectedAvail, outStr)
	}
	if !strings.Contains(outStr, "<D:quota-used-bytes>100</D:quota-used-bytes>") {
		t.Errorf("expected quota-used-bytes 100, got: %s", outStr)
	}

	// Case 4: mixed 404 propstat (other unknown prop retained in 404)
	inputMixed := `<?xml version="1.0" encoding="UTF-8"?><D:multistatus xmlns:D="DAV:"><D:response><D:href>/webdav/</D:href><D:propstat><D:prop><D:displayname/></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat><D:propstat><D:prop><D:other-prop/><D:quota-available-bytes/></D:prop><D:status>HTTP/1.1 404 Not Found</D:status></D:propstat></D:response></D:multistatus>`
	out, err = InjectQuotaProperties([]byte(inputMixed), 500, 100, true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outStr = string(out)
	if !strings.Contains(outStr, "404 Not Found") || !strings.Contains(outStr, "<D:other-prop/>") {
		t.Errorf("expected 404 with other-prop retained, got: %s", outStr)
	}
	if !strings.Contains(outStr, "<D:quota-available-bytes>400</D:quota-available-bytes>") {
		t.Errorf("expected 200 with quota-available-bytes 400, got: %s", outStr)
	}
	if strings.Contains(outStr, "quota-used-bytes") {
		t.Errorf("quota-used-bytes was not requested and should not be present, got: %s", outStr)
	}

	// Case 5: Depth 1 with multiple responses
	inputMulti := `<?xml version="1.0" encoding="UTF-8"?><D:multistatus xmlns:D="DAV:"><D:response><D:href>/webdav/</D:href><D:propstat><D:prop><D:displayname/></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response><D:response><D:href>/webdav/file.txt</D:href><D:propstat><D:prop><D:displayname>file.txt</D:displayname></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response></D:multistatus>`
	out, err = InjectQuotaProperties([]byte(inputMulti), 1000, 300, true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outStr = string(out)
	if strings.Count(outStr, "<D:quota-available-bytes>700</D:quota-available-bytes>") != 2 {
		t.Errorf("expected 2 instances of quota-available-bytes 700, got: %s", outStr)
	}
	if strings.Count(outStr, "<D:quota-used-bytes>300</D:quota-used-bytes>") != 2 {
		t.Errorf("expected 2 instances of quota-used-bytes 300, got: %s", outStr)
	}
}
