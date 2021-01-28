// Copyright 2021 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package feishurobot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Notifier implements a Notifier for feishurobot notifications.
type Notifier struct {
	conf   *config.FeishuRobotConfig
	tmpl   *template.Template
	logger log.Logger
	client *http.Client
}

func New(c *config.FeishuRobotConfig, t *template.Template, l log.Logger) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "feishurobot", false, false)
	if err != nil {
		return nil, err
	}

	return &Notifier{conf: c, tmpl: t, logger: l, client: client}, nil
}

type feishuRobotMsg struct {
	MsgType string            `yaml:"msg_type,omitempty" json:"msg_type,omitempty"`
	Content feishuTextContent `yaml:"content,omitempty" json:"content,omitempty"`
}

type feishuTextContent struct {
	Text string `yaml:"text,omitempty" json:"text,omitempty"`
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
	}

	level.Debug(n.logger).Log("incident", key)
	data := notify.GetTemplateData(ctx, n.tmpl, as, n.logger)

	tmpl := notify.TmplText(n.tmpl, data, &err)
	if err != nil {
		return false, err
	}

	msg := &feishuRobotMsg{
		MsgType: "text",
		Content: feishuTextContent{
			Text: tmpl(n.conf.Message),
		},
	}

	if err != nil {
		return false, fmt.Errorf("templating error: %s", err)
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return false, err
	}

	postMessageURL := n.conf.APIURL.Copy()

	req, err := http.NewRequest(http.MethodPost, postMessageURL.String(), &buf)
	if err != nil {
		return true, err
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := n.client.Do(req.WithContext(ctx))
	if err != nil {
		return true, notify.RedactURL(err)
	}
	defer notify.Drain(resp)

	if resp.StatusCode != 200 {
		return true, fmt.Errorf("unexpected status code %v", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return true, err
	}
	level.Debug(n.logger).Log("response", string(body), "incident", key)

	return false, nil
}
