package router

import (
	"strings"
	"testing"
)

func TestSubmitFunctionBodiesMaskRenderedValues(t *testing.T) {
	document := `<input id="wlKeys1" name="wlKeys1" type="password">
<script>
var current = 'HomeNet-5G';
function SubmitForm() {
	var Form = new webSubmitForm();
	if (getValue('wlKeys1') == 'SecretKey123') { return; } // brace in comment }
	Form.addParameter('x.PreSharedKey', getValue('wlKeys1'));
	Form.addParameter('x.WPSPin', 12345670);
	Form.addParameter('x.Retries', 3);
	Form.setAction('set.cgi?x=InternetGatewayDevice.LANDevice.1.WLANConfiguration.1&RequestFile=html/amp/wlanbasic/WlanBasic.asp');
	Form.setAction('set.cgi?y=' + node + '&RequestFile=html/amp/wlanbasic/WlanBasic.asp');
	Form.submit();
}
function LoadFrame() { setText('ssid', "HomeNet-5G"); }
</script>`
	bodies := submitFunctionBodies(document, pageControlNames(document))
	body, ok := bodies["SubmitForm"]
	if !ok {
		t.Fatalf("SubmitForm not captured: %v", bodies)
	}
	if _, ok := bodies["LoadFrame"]; ok {
		t.Fatalf("LoadFrame has no submitter signal and should be skipped")
	}
	for _, secret := range []string{"SecretKey123", "HomeNet-5G", "12345670"} {
		if strings.Contains(body, secret) {
			t.Fatalf("submitter body contains rendered value %q:\n%s", secret, body)
		}
	}
	for _, kept := range []string{"'x.PreSharedKey'", "getValue('wlKeys1')", "'x.Retries', 3)", "set.cgi?x=InternetGatewayDevice.LANDevice.1.WLANConfiguration.1", "+ '&RequestFile=html/amp/wlanbasic/WlanBasic.asp'", "Form.submit();\n}"} {
		if !strings.Contains(body, kept) {
			t.Fatalf("submitter body lost %q:\n%s", kept, body)
		}
	}
}
