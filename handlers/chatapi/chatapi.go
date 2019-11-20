package chatapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/nyaruka/gocommon/urns"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/go-errors/errors"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("CA"), "Chat API")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	return nil
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// no message? ignore this
	if payload.InstanceID == "" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no message")
	}

	// the list of events we deal with
	events := make([]courier.Event, 0, 2)

	// the list of data we will return in our response
	data := make([]interface{}, 0, 2)

	for i := range payload.Messages {
		message := payload.Messages[i]

		if message.FromMe == false {
			// create our date from the timestamp
			date := time.Unix(message.Time, 0).UTC()

			// create our URN
			author := message.Author
			contactPhoneNumber := strings.Replace(author, "@c.us", "", 1)
			urn, errURN := urns.NewWhatsAppURN(contactPhoneNumber)
			if errURN != nil {
				return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errURN)
			}

			// build our name from first and last
			name := handlers.NameFromFirstLastUsername(message.SenderName, "", "")

			// our text is either "text" or "caption" (or empty)
			text := message.Body
			isAttachment := false
			if message.Type == "image" {
				text = message.Caption
				isAttachment = true
			}

			// build our msg
			ev := h.Backend().NewIncomingMsg(channel, urn, text).WithExternalID(message.ID).WithReceivedOn(date).WithContactName(name)
			event := h.Backend().CheckExternalIDSeen(ev)

			if isAttachment {
				event.WithAttachment(message.Body)
			}

			errMsg := h.Backend().WriteMsg(ctx, event)
			if errMsg != nil {
				return nil, errMsg
			}

			h.Backend().WriteExternalIDSeen(event)

			events = append(events, event)
			data = append(data, courier.NewMsgReceiveData(event))
		}
	}

	for i := range payload.Ack {
		ack := payload.Ack[i]
		status := courier.MsgQueued

		if ack.Status == "sent" {
			status = courier.MsgSent
		} else if ack.Status == "delivered" {
			status = courier.MsgDelivered
		}

		event := h.Backend().NewMsgStatusForExternalID(channel, ack.ID, status)
		err := h.Backend().WriteMsgStatus(ctx, event)

		// we don't know about this message, just tell them we ignored it
		if err == courier.ErrMsgNotFound {
			data = append(data, courier.NewInfoData("message not found, ignored"))
			continue
		}

		if err != nil {
			return nil, err
		}

		events = append(events, event)
		data = append(data, courier.NewStatusData(event))

	}

	return events, courier.WriteDataResponse(ctx, w, http.StatusOK, "Events Handled", data)

}

func (h *handler) sendMsgPart(msg courier.Msg, apiURL string, token string, path string, payload interface{}) (string, *courier.ChannelLog, error) {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		log := courier.NewChannelLog("unable to build JSON body", msg.Channel(), msg.ID(), "", "", courier.NilStatusCode, "", "", time.Duration(0), err)
		return "", log, err
	}

	sendURL := fmt.Sprintf("%s/%s?token=%s", apiURL, path, token)
	req, _ := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rr, err := utils.MakeHTTPRequest(req)

	// build our channel log
	log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)

	externalID, err := jsonparser.GetString(rr.Body, "id")
	if err != nil {
		log.WithError("Message Send Error", errors.Errorf("unable to get id from body"))
		return "", log, errors.Errorf("no 'id' in response")
	}

	return externalID, log, nil
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	confAuth := msg.Channel().ConfigForKey(courier.ConfigAuthToken, "")
	authToken, isStr := confAuth.(string)
	if !isStr || authToken == "" {
		return nil, fmt.Errorf("invalid auth token config")
	}

	confSendURL := msg.Channel().ConfigForKey(courier.ConfigSendURL, "")
	sendURL, isStr := confSendURL.(string)
	if !isStr || sendURL == "" {
		return nil, fmt.Errorf("invalid send url config")
	}

	// we only caption if there is only a single attachment
	caption := ""
	if len(msg.Attachments()) == 1 {
		caption = msg.Text()
	}

	// the status that will be written for this message
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	// whether we encountered any errors sending any parts
	hasError := true

	// if we have text, send that if we aren't sending it as a caption
	if msg.Text() != "" && caption == "" {
		payload := moSendMsgPayload{
			Phone: msg.URN().Path(),
			Body:  msg.Text(),
		}

		externalID, log, err := h.sendMsgPart(msg, sendURL, authToken, "sendMessage", payload)
		status.SetExternalID(externalID)
		hasError = err != nil
		status.AddLog(log)
	}

	// send each attachment
	for _, attachment := range msg.Attachments() {
		mediaType, mediaURL := handlers.SplitAttachment(attachment)
		switch strings.Split(mediaType, "/")[0] {
		case "image":
			payload := moSendFilePayload{
				Phone:    msg.URN().Path(),
				Body:     mediaURL,
				Filename: "file.jpg",
				Caption:  caption,
			}
			externalID, log, err := h.sendMsgPart(msg, sendURL, authToken, "sendFile", payload)
			status.SetExternalID(externalID)
			hasError = err != nil
			status.AddLog(log)

		default:
			status.AddLog(courier.NewChannelLog("Unknown media type: "+mediaType, msg.Channel(), msg.ID(), "", "", courier.NilStatusCode,
				"", "", time.Duration(0), fmt.Errorf("unknown media type: %s", mediaType)))
			hasError = true

		}
	}

	if !hasError {
		status.SetStatus(courier.MsgWired)
	}

	return status, nil
}

//{
//	"instanceId": "79926",
//	"messages": [
//		{
//			"id": "false_17472822486@c.us_DF38E6A25B42CC8CCE57EC40F",
//			"body": "Ok!",
//			"type": "chat",
//			"senderName": "Ilya",
//			"fromMe": true,
//			"author": "17472822486@c.us",
//			"time": 1504208593,
//			"chatId": "17472822486@c.us",
//			"messageNumber": 100
//		}
//	]
//}
// Or
//{
//	"instanceId": "79926",
//	"ack": [
//		{
//			"id": "false_17472822486@c.us_DF38E6A25B42CC8CCE57EC40F",
//			"messageNumber": 100,
//			"chatId": "17472822486@c.us",
//			"status": "delivered"
//		}
//	]
//}
type moPayload struct {
	InstanceID string      `json:"instanceId" validate:"required"`
	Messages   []moMessage `json:"messages"`
	Ack        []moAck     `json:"ack"`
}

type moAck struct {
	ID            string `json:"id"`
	MessageNumber int    `json:"messageNumber"`
	ChatID        string `json:"chatId"`
	Status        string `json:"status"`
}

type moMessage struct {
	ID            string `json:"id"`
	Body          string `json:"body"`
	Type          string `json:"type"`
	SenderName    string `json:"senderName"`
	FromMe        bool   `json:"fromMe"`
	Author        string `json:"author"`
	Time          int64  `json:"time"`
	ChatID        string `json:"chatId"`
	MessageNumber int    `json:"messageNumber"`
	Caption       string `json:"caption"`
}

//{
//	"phone": "79995253422",
//	"body": "WhatsApp API on chat-api.com works good"
//}
type moSendMsgPayload struct {
	Phone string `json:"phone"`
	Body  string `json:"body"`
}

//{
//	"phone": "79995253422",
//	"body": "https://path.to.file.com/file.jpg",
//	"caption": "WhatsApp API on chat-api.com works good",
//	"filename": "file.jpg"
//}
type moSendFilePayload struct {
	Phone    string `json:"phone"`
	Body     string `json:"body"`
	Filename string `json:"filename"`
	Caption  string `json:"caption"`
}
