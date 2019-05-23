package rapidpro

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/celery"
)

func queueTask(rc redis.Conn, queueName string, taskName string, orgID OrgID, subQueue string, body map[string]interface{}) (err error) {
	// encode our body
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	epochFloat := float64(now.UnixNano()) / float64(time.Second)

	// we do all our queueing in a transaction
	rc.Send("multi")
	if subQueue != "" {
		rc.Send("zadd", subQueue, fmt.Sprintf("%.5f", epochFloat), bodyJSON)
	}
	rc.Send("zadd", fmt.Sprintf("%s:%d", taskName, orgID), fmt.Sprintf("%.5f", epochFloat-10000000), bodyJSON)
	rc.Send("zincrby", fmt.Sprintf("%s:active", taskName), 0, orgID)
	celery.QueueEmptyTask(rc, queueName, taskName)
	_, err = rc.Do("exec")

	return err
}

func queueMsgHandling(rc redis.Conn, c *DBContact, m *DBMsg) error {
	channel := m.Channel().(*DBChannel)

	// flow server enabled orgs go to mailroom
	if channel.OrgFlowServerEnabled() {
		body := map[string]interface{}{
			"contact_id":      c.ID_,
			"org_id":          channel.OrgID_,
			"channel_id":      channel.ID_,
			"msg_id":          m.ID_,
			"msg_uuid":        m.UUID_.String(),
			"msg_external_id": m.ExternalID(),
			"urn":             m.URN().String(),
			"urn_id":          m.ContactURNID_,
			"text":            m.Text(),
			"attachments":     m.Attachments(),
			"new_contact":     c.IsNew_,
		}

		return queueMailroomTask(rc, "msg_event", m.OrgID_, m.ContactID_, body)

	}

	body := map[string]interface{}{
		"type":        "msg",
		"id":          m.ID_,
		"contact_id":  c.ID_,
		"new_message": true,
		"new_contact": c.IsNew_,
	}

	return queueTask(rc, "handler", "handle_event_task", m.OrgID_, fmt.Sprintf("ch:%d", c.ID_), body)
}

func queueChannelEvent(rc redis.Conn, c *DBContact, e *DBChannelEvent) error {
	channel := e.Channel()

	// flow server enabled orgs go to mailroom
	if channel.OrgFlowServerEnabled() {
		switch e.EventType() {
		case courier.StopContact:
			body := map[string]interface{}{
				"org_id":     e.OrgID_,
				"contact_id": e.ContactID_,
			}
			return queueMailroomTask(rc, "stop_event", e.OrgID_, e.ContactID_, body)

		case courier.WelcomeMessage:
			body := map[string]interface{}{
				"org_id":      e.OrgID_,
				"contact_id":  e.ContactID_,
				"urn_id":      e.ContactURNID_,
				"channel_id":  e.ChannelID_,
				"new_contact": c.IsNew_,
			}
			return queueMailroomTask(rc, "welcome_message", e.OrgID_, e.ContactID_, body)

		case courier.Referral:
			body := map[string]interface{}{
				"org_id":      e.OrgID_,
				"contact_id":  e.ContactID_,
				"urn_id":      e.ContactURNID_,
				"channel_id":  e.ChannelID_,
				"extra":       e.Extra(),
				"new_contact": c.IsNew_,
			}
			return queueMailroomTask(rc, "referral", e.OrgID_, e.ContactID_, body)

		case courier.NewConversation:
			body := map[string]interface{}{
				"org_id":      e.OrgID_,
				"contact_id":  e.ContactID_,
				"urn_id":      e.ContactURNID_,
				"channel_id":  e.ChannelID_,
				"extra":       e.Extra(),
				"new_contact": c.IsNew_,
			}
			return queueMailroomTask(rc, "new_conversation", e.OrgID_, e.ContactID_, body)

		default:
			return fmt.Errorf("unknown event type: %s", e.EventType())
		}
	}

	body := map[string]interface{}{
		"type":       "channel_event",
		"contact_id": e.ContactID_,
		"event_id":   e.ID_,
	}

	return queueTask(rc, "handler", "handle_event_task", e.OrgID_, "", body)
}

// queueMailroomTask queues the passed in task to mailroom. Mailroom processes both messages and
// channel event tasks through the same ordered queue.
func queueMailroomTask(rc redis.Conn, taskType string, orgID OrgID, contactID ContactID, body map[string]interface{}) (err error) {
	// create our event task
	eventTask := mrTask{
		Type:     taskType,
		OrgID:    orgID,
		Task:     body,
		QueuedOn: time.Now(),
	}

	eventJSON, err := json.Marshal(eventTask)
	if err != nil {
		return err
	}

	// create our org task
	contactTask := mrTask{
		Type:  "handle_contact_event",
		OrgID: orgID,
		Task: mrContactTask{
			ContactID: contactID,
		},
		QueuedOn: time.Now(),
	}

	contactJSON, err := json.Marshal(contactTask)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	epochFloat := float64(now.UnixNano()) / float64(time.Second)

	// we do all our queueing in a transaction
	contactQueue := fmt.Sprintf("c:%d:%d", orgID, contactID)
	rc.Send("multi")
	rc.Send("rpush", contactQueue, eventJSON)
	rc.Send("zadd", fmt.Sprintf("handler:%d", orgID), fmt.Sprintf("%.5f", epochFloat-10000000), contactJSON)
	rc.Send("zincrby", "handler:active", 0, orgID)
	_, err = rc.Do("exec")

	return err
}

type mrContactTask struct {
	ContactID ContactID `json:"contact_id"`
}

type mrTask struct {
	Type     string      `json:"type"`
	OrgID    OrgID       `json:"org_id"`
	Task     interface{} `json:"task"`
	QueuedOn time.Time   `json:"queued_on"`
}
