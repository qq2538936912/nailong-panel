package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"panel/service"
)

const panelGitHubRepo = "qq2538936912/nailong-panel"
const latestPanelReleaseAPI = "https://api.github.com/repos/" + panelGitHubRepo + "/releases/latest"

type panelReleaseInfo struct {
	TagName     string              `json:"tag_name"`
	Name        string              `json:"name"`
	Body        string              `json:"body"`
	HTMLURL     string              `json:"html_url"`
	PublishedAt string              `json:"published_at"`
	Assets      []panelReleaseAsset `json:"assets"`
}

type panelReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func (r panelReleaseInfo) version() string {
	return strings.TrimPrefix(strings.TrimSpace(r.TagName), "v")
}

func (r panelReleaseInfo) findAsset(name string) (panelReleaseAsset, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return panelReleaseAsset{}, false
	}

	for _, asset := range r.Assets {
		if strings.EqualFold(strings.TrimSpace(asset.Name), name) {
			return asset, true
		}
	}
	return panelReleaseAsset{}, false
}

func fetchLatestPanelRelease() (*panelReleaseInfo, error) {
	client := service.NewHTTPClient(15 * time.Second)
	resp, err := client.Get(latestPanelReleaseAPI)
	if err != nil {
		return nil, fmt.Errorf("检查更新失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("当前 GitHub 仓库还没有 Release")
		}
		return nil, fmt.Errorf("GitHub API 返回异常状态")
	}

	var release panelReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析 GitHub 响应失败")
	}

	return &release, nil
}
