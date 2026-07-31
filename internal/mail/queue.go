package mail

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"sanalcp/internal/httpx"
)

type QueueRecipient struct {
	Address     string `json:"address"`
	DelayReason string `json:"delay_reason,omitempty"`
}

type QueueMessage struct {
	QueueID     string           `json:"queue_id"`
	QueueName   string           `json:"queue_name"`
	ArrivalTime int64            `json:"arrival_time"`
	MessageSize int64            `json:"message_size"`
	Sender      string           `json:"sender"`
	Recipients  []QueueRecipient `json:"recipients"`
}

var queueIDRE = regexp.MustCompile(`^[A-Za-z0-9]{5,64}$`)

// QueueList: GET /admin/mail/queue
func (h *Handlers) QueueList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "postqueue", "-j")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "Postfix kuyruğu okunamadı: "+err.Error())
		return
	}
	out := make([]QueueMessage, 0)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var item QueueMessage
		if err := json.Unmarshal(scanner.Bytes(), &item); err == nil {
			out = append(out, item)
		}
	}
	waitErr := cmd.Wait()
	if scanner.Err() != nil || waitErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" && waitErr != nil {
			detail = waitErr.Error()
		}
		httpx.WriteError(w, http.StatusServiceUnavailable, "Postfix kuyruğu okunamadı: "+detail)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"messages": out, "count": len(out)})
}

// QueueAction: POST /admin/mail/queue {action, queue_id?}
func (h *Handlers) QueueAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action  string `json:"action"`
		QueueID string `json:"queue_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	var name string
	var args []string
	switch req.Action {
	case "flush":
		name, args = "postqueue", []string{"-f"}
	case "delete", "hold", "release", "requeue":
		if !queueIDRE.MatchString(req.QueueID) {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz kuyruk kimliği")
			return
		}
		flag := map[string]string{"delete": "-d", "hold": "-h", "release": "-H", "requeue": "-r"}[req.Action]
		name, args = "postsuper", []string{flag, req.QueueID}
	default:
		httpx.WriteError(w, http.StatusBadRequest, "action: flush|delete|hold|release|requeue")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError,
			fmt.Sprintf("kuyruk işlemi: %s", strings.TrimSpace(string(out))))
		return
	}
	h.audit(r, "mail.queue."+req.Action, req.QueueID, true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "action": req.Action, "queue_id": req.QueueID,
	})
}
