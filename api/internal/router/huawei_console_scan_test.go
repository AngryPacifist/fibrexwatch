package router

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDescribeConsolePageOmitsRenderedValues(t *testing.T) {
	document := `<form id="ConfigForm" action="set.cgi" method="post">
<input type="password" id="wlKeys" name="wlKeys" value="SecretKey123" onclick="fill('SecretKey123')">
<input type="hidden" name="x.X_HW_Token" value="tok-abc">
</form>
<script>
var WlanList = new Array(new stWlanInfo("InternetGatewayDevice.LANDevice.1.WLANConfiguration.1","HomeNet-5G","SecretKey123"),null);
function stWlanInfo(domain, ssid, key) {}
function SubmitForm() { Form.addParameter('x.PreSharedKey', getValue('wlKeys')); Form.setAction('set.cgi?x=InternetGatewayDevice.LANDevice.1.WLANConfiguration.1&RequestFile=html/amp/wlanbasic/WlanBasic.asp'); }
</script>`
	page := ConsolePage{Path: "/html/amp/wlanbasic/WlanBasic.asp"}
	describeConsolePage(&page, document)
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SecretKey123", "HomeNet-5G", "tok-abc"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("scan output contains rendered value %q: %s", secret, encoded)
		}
	}
	if page.Records["stWlanInfo"] != 1 {
		t.Fatalf("records = %v, want stWlanInfo counted once", page.Records)
	}
	if !containsPath(page.FieldKeys, "x.PreSharedKey") {
		t.Fatalf("field keys = %v, want x.PreSharedKey", page.FieldKeys)
	}
	if !containsPath(page.Functions, "stWlanInfo(domain,ssid,key)") {
		t.Fatalf("functions = %v", page.Functions)
	}
	if len(page.Endpoints) != 2 || page.Endpoints[0] != "set.cgi" || !strings.HasPrefix(page.Endpoints[1], "set.cgi?x=InternetGatewayDevice.LANDevice.1.WLANConfiguration.1") {
		t.Fatalf("endpoints = %v", page.Endpoints)
	}
}

func TestScannablePageSkipsActionsAndLogout(t *testing.T) {
	for path, want := range map[string]bool{
		"/html/amp/wlanbasic/WlanBasic.asp": true,
		"/html/ssmp/reset/reset.asp":        true,
		"/logout.cgi":                       false,
		"/html/bbsp/wlanmacfilter/set.cgi":  false,
		"/html/logout.asp":                  false,
		"/login.asp":                        false,
		"/resource/common/util.js":          false,
		"relative.asp":                      false,
	} {
		if got := scannablePage(path); got != want {
			t.Errorf("scannablePage(%q) = %t, want %t", path, got, want)
		}
	}
}
