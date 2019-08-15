package freshchat

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "FC", "2020", "US", map[string]interface{}{
		"username":   "c8fddfaf-622a-4a0e-b060-4f3ccbeab606", //agent_id
		"secret":     cert,                                   // public_key for sig
		"auth_token": "authtoken",                            //API bearer token
	}),
	// author-id
}

var (
	cert = "-----BEGIN RSA PUBLIC KEY----- MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAuGJLF4hTTtxWogT6dNkGf3CEgLAR2mGJzlds5cNbrHFoJNFnmVhkRYGzLYxx4EtDiezNCZVHfyMI2AKuNSQW2fEdDatVIG+q3Zr/X9eeDl8kQOGy804J/fgCYDrN8RQu0n5Dh1inv4puca0wb29SCvoAwrWb33ehDBIvv6+rUKBdjtv2xTV65kNiVDo5VRCaYRVeE10osxeONgw55HVY4nczuxnR+dmc2282de6WHe5LXtr0ZBdJ8yttFOLIluZ/sNM5DIWZBkIWQhyT581tbA7bTpsIbrT/IMBlmioIILw8WGtI7zcmNkjU5dnq5HnlVKEDhj/Ug/dLiyno8+Vp7QIDAQAB -----END RSA PUBLIC KEY-----"

	receiveURL       = "/c/fc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	notJSON          = "empty"
	validSignature   = `AhrmypOSWoewHG6LmIRuWjxyokuMDmPklrSU9p0gpUNjdSRCJzvpL6rjuTi5poV/ZLzWRWNM7X9yWjT5m9YFPshYrvigcd1ph4Ot2xmaJGYoUNJHijQccE6oDtDIp6i/8oLRafHgObQnGukZWPbP9OE5EiKz/VcsMP0Wv7hawI/sfIviM0w+6fNOKXWi0jDBH9ap1mj5CqOUOojni7OD5iYmIrjV/h33dyNmbvAta9E+trzcEhYqxfHIN4Z8R2FsatfRHWicoQ4PE5cQ8+UONVya8qr85nQ9w8N7Ql7yNg9fEViYG4/W/JnGEbPPEf8WrYtKzoVyuupDz4mVHdfKWg==`
	validReceive     = `{"actor":{"actor_type":"user","actor_id":"882f3926-b292-414b-a411-96380db373cd"},"action":"message_create","action_time":"2019-06-21T17:43:20.875Z","data":{"message":{"message_parts":[{"text":{"content":"Test 2"}}],"app_id":"55b190fa-5d3c-45c4-bc49-74ddcfcf53d7","actor_id":"882f3926-b292-414b-a411-96380db373cd","id":"7a454fde-c720-4c97-a61d-0ffe70449eb6","channel_id":"c8fddfaf-622a-4a0e-b060-4f3ccbeab606","conversation_id":"c327498e-f713-481e-8d83-0603e03d2521","message_type":"normal","actor_type":"user","created_time":"2019-06-21T17:43:20.866Z"}}}`
	invalidSignature = `f7wMD1BBhcj60U0z3dCY519qmxQ8qfVUU212Dapw9vpZfRBfjjmukUK2GwbAb0Nc+TGQHxN4iP4WD+Y/mSx6f4bmkBsvCy3l4OCQ/FEK0y5R7f+GLLDhgbTh90MwuLDHhvxB5dxIeu59leL+4yO+l/8M3Tm48aQurVBi9IAlzFsMtc1S1CiRxsDUb/rD6IRekPa0pUAbkno9qJ/CGXh0kZMdsYzRkzZmKCs79OWrvU94ha0ptyt5wArfmD1oSzY3PjeL2w8LWDc0QV21H/Hvj42azIUqebiNRtZ2E+f34AfQsyfcPuy1k/6qLuYGOdU1uZidPuPcGpeSIm0GW6k9HQ==`
	invalidURN       = `{"actor":{"actor_type":"user","actor_id":"c0534ff79-8853-11cedfc1f35b"},"action":"message_create","action_time":"2019-06-21T14:21:35.042Z","data":{"message":{"message_parts":[{"text":{"content":"test"}}],"app_id":"55b190fa-5d3c-45c4-bc49-74ddcfcf53d7","actor_id":"c0534f78-b6e9-4f79-8853-11cedfc1f35b","id":"3fce6f90-a01a-44a9-8ab1-8feea6ebc95b","channel_id":"c8fddfaf-622a-4a0e-b060-4f3ccbeab606","conversation_id":"c327498e-f713-481e-8d83-0603e03d2521","message_type":"normal","actor_type":"user","created_time":"2019-06-21T14:21:35Z"}}}`
)
var sigtestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid w Signature",
		Headers: map[string]string{
			"Content-Type":          "application/json",
			"X-FreshChat-Signature": validSignature},
		URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Test 2"), URN: Sp("freshchat:c8fddfaf-622a-4a0e-b060-4f3ccbeab606/882f3926-b292-414b-a411-96380db373cd"), Date: Tp(time.Date(2019, 6, 21, 17, 43, 20, 866000000, time.UTC))},

	{Label: "Bad Signature",
		Headers: map[string]string{
			"Content-Type":          "application/json",
			"X-FreshChat-Signature": invalidSignature},
		URL: receiveURL, Data: validReceive, Status: 400, Response: `{"message":"Error","data":[{"type":"error","error":"unable to verify signature, crypto/rsa: verification error"}]}`,
		Text: Sp("Test 2"), URN: Sp("freshchat:c8fddfaf-622a-4a0e-b060-4f3ccbeab606/882f3926-b292-414b-a411-96380db373cd"), Date: Tp(time.Date(2019, 6, 21, 17, 43, 20, 866000000, time.UTC))},
}
var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid w Sig",
		Headers: map[string]string{
			"Content-Type":          "application/json",
			"X-FreshChat-Signature": validSignature},
		URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Test 2"), URN: Sp("freshchat:c8fddfaf-622a-4a0e-b060-4f3ccbeab606/882f3926-b292-414b-a411-96380db373cd"), Date: Tp(time.Date(2019, 6, 21, 17, 43, 20, 866000000, time.UTC))},

	{Label: "Bad JSON",
		Headers: map[string]string{
			"Content-Type":          "application/json",
			"X-FreshChat-Signature": invalidSignature},
		URL: receiveURL, Data: notJSON, Status: 400, Response: `{"message":"Error","data":[{"type":"error","error":"unable to parse request JSON: invalid character 'e' looking for beginning of value"}]}`,
		Text: Sp("Test 2"), URN: Sp("freshchat:c8fddfaf-622a-4a0e-b060-4f3ccbeab606/882f3926-b292-414b-a411-96380db373cd"), Date: Tp(time.Date(2019, 6, 21, 17, 43, 20, 866000000, time.UTC))},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler("FC", "FreshChat", true), sigtestCases)
	RunChannelTestCases(t, testChannels, newHandler("FC", "FreshChat", false), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler("FC", "FreshChat", false), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	apiURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text:           "Simple Message ☺",
		URN:            "freshchat:0534f78-b6e9-4f79-8853-11cedfc1f35b/c8fddfaf-622a-4a0e-b060-4f3ccbeab606",
		Status:         "W",
		ExternalID:     "",
		ResponseBody:   "",
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer enYtdXNlcm5hbWU6enYtcGFzc3dvcmQ=",
		},
		RequestBody: `{"messages":[{"message_parts":[{"text":{"content":"Simple Message ☺"}}],"actor_id":"c8fddfaf-622a-4a0e-b060-4f3ccbeab606","actor_type":"agent"}],"channel_id":"0534f78-b6e9-4f79-8853-11cedfc1f35b","users":[{"id":"c8fddfaf-622a-4a0e-b060-4f3ccbeab606"}]}`,
		SendPrep:    setSendURL,
	},
	{Label: "Send with text and image",
		Text:           "Simple Message ☺",
		URN:            "freshchat:0534f78-b6e9-4f79-8853-11cedfc1f35b/c8fddfaf-622a-4a0e-b060-4f3ccbeab606",
		Status:         "W",
		ExternalID:     "",
		ResponseBody:   "",
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer enYtdXNlcm5hbWU6enYtcGFzc3dvcmQ=",
		},
		Attachments: []string{"https://foo.bar/image.jpg"},
		RequestBody: `{"messages":[{"message_parts":[{"text":{"content":"Simple Message ☺"}},{"image":{"url":"https://foo.bar/image.jpg"}}],"actor_id":"c8fddfaf-622a-4a0e-b060-4f3ccbeab606","actor_type":"agent"}],"channel_id":"0534f78-b6e9-4f79-8853-11cedfc1f35b","users":[{"id":"c8fddfaf-622a-4a0e-b060-4f3ccbeab606"}]}`,
		SendPrep:    setSendURL,
	},
	{Label: "Send with image only",
		URN:            "freshchat:0534f78-b6e9-4f79-8853-11cedfc1f35b/c8fddfaf-622a-4a0e-b060-4f3ccbeab606",
		Status:         "W",
		ExternalID:     "",
		ResponseBody:   "",
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer enYtdXNlcm5hbWU6enYtcGFzc3dvcmQ=",
		},
		Attachments: []string{"https://foo.bar/image.jpg"},
		RequestBody: `{"messages":[{"message_parts":[{"image":{"url":"https://foo.bar/image.jpg"}}],"actor_id":"c8fddfaf-622a-4a0e-b060-4f3ccbeab606","actor_type":"agent"}],"channel_id":"0534f78-b6e9-4f79-8853-11cedfc1f35b","users":[{"id":"c8fddfaf-622a-4a0e-b060-4f3ccbeab606"}]}`,
		SendPrep:    setSendURL,
	},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "FC", "2020", "US", map[string]interface{}{
		"username":   "c8fddfaf-622a-4a0e-b060-4f3ccbeab606",
		"secret":     cert,
		"auth_token": "enYtdXNlcm5hbWU6enYtcGFzc3dvcmQ=",
	})
	RunChannelSendTestCases(t, defaultChannel, newHandler("FC", "FreshChat", false), defaultSendTestCases, nil)
}
