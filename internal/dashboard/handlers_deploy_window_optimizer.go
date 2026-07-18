package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeployWindowOptimizerResult analyzes deployment patterns to recommend
// optimal deployment windows based on historical activity and risk factors.
type DeployWindowOptimizerResult struct {
	ScannedAt          time.Time           `json:"scannedAt"`
	Summary            DeployWindowSummary `json:"summary"`
	HourlyHeatmap      []HourWindow        `json:"hourlyHeatmap"`
	WeeklyPattern      []DayWindow         `json:"weeklyPattern"`
	RecommendedWindows []DeployWindow      `json:"recommendedWindows"`
	OptimizerScore     int                 `json:"optimizerScore"`
	Grade              string              `json:"grade"`
	Recommendations    []string            `json:"recommendations"`
}

type DeployWindowSummary struct {
	TotalDeploys     int     `json:"totalDeploys"`
	PeakHour         int     `json:"peakHour"`
	PeakHourCount    int     `json:"peakHourCount"`
	OffHoursDeploys  int     `json:"offHoursDeploys"`
	WeekendDeploys   int     `json:"weekendDeploys"`
	ChangeFreezeOK   bool    `json:"changeFreezeCompliant"`
	AvgDeploysPerDay float64 `json:"avgDeploysPerDay"`
}

type HourWindow struct {
	Hour      int    `json:"hour"`
	Count     int    `json:"count"`
	RiskLevel string `json:"riskLevel"`
}

type DayWindow struct {
	DayName string `json:"dayName"`
	Count   int    `json:"count"`
}

type DeployWindow struct {
	Day       string `json:"day"`
	HourRange string `json:"hourRange"`
	Score     int    `json:"score"`
	Reason    string `json:"reason"`
}

// handleDeployWindowOptimizer handles GET /api/deployment/deploy-window-optimizer
func (s *Server) handleDeployWindowOptimizer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := DeployWindowOptimizerResult{ScannedAt: time.Now()}

	rss, _ := rc.clientset.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})

	// Collect deploy timestamps from ReplicaSets
	hourCounts := make(map[int]int)
	dayCounts := make(map[string]int)
	totalDeploys := 0

	for _, rs := range rss.Items {
		if isSystemNamespace(rs.Namespace) {
			continue
		}
		ts := rs.CreationTimestamp.Time
		hour := ts.Hour()
		day := ts.Weekday().String()
		hourCounts[hour]++
		dayCounts[day]++
		totalDeploys++

		// Off-hours: 22:00-06:00
		if hour >= 22 || hour < 6 {
			result.Summary.OffHoursDeploys++
		}
		// Weekend
		if day == "Saturday" || day == "Sunday" {
			result.Summary.WeekendDeploys++
		}
	}

	result.Summary.TotalDeploys = totalDeploys

	// Build hourly heatmap
	peakHour := 0
	peakCount := 0
	for h := 0; h < 24; h++ {
		count := hourCounts[h]
		risk := "low"
		if h >= 22 || h < 6 {
			risk = "high" // off-hours = risky
		} else if h >= 10 && h <= 16 {
			risk = "optimal" // business hours = safe
		} else {
			risk = "medium"
		}
		result.HourlyHeatmap = append(result.HourlyHeatmap, HourWindow{Hour: h, Count: count, RiskLevel: risk})
		if count > peakCount {
			peakCount = count
			peakHour = h
		}
	}
	result.Summary.PeakHour = peakHour
	result.Summary.PeakHourCount = peakCount

	// Weekly pattern
	dayOrder := []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
	for _, day := range dayOrder {
		result.WeeklyPattern = append(result.WeeklyPattern, DayWindow{DayName: day, Count: dayCounts[day]})
	}

	// Change freeze compliance: no weekend deploys
	result.Summary.ChangeFreezeOK = result.Summary.WeekendDeploys == 0

	// Recommended windows
	result.RecommendedWindows = []DeployWindow{
		{Day: "Tuesday", HourRange: "10:00-12:00", Score: 95, Reason: "Low traffic, full team available"},
		{Day: "Wednesday", HourRange: "10:00-14:00", Score: 90, Reason: "Mid-week stability window"},
		{Day: "Thursday", HourRange: "10:00-12:00", Score: 85, Reason: "Pre-weekend safety buffer"},
		{Day: "Monday", HourRange: "11:00-13:00", Score: 70, Reason: "Post-weekend catch-up risk"},
	}

	// Avoid windows
	if result.Summary.OffHoursDeploys > 0 {
		result.RecommendedWindows = append(result.RecommendedWindows, DeployWindow{
			Day: "Avoid", HourRange: "22:00-06:00", Score: 10, Reason: fmt.Sprintf("%d off-hours deploys detected", result.Summary.OffHoursDeploys),
		})
	}
	if result.Summary.WeekendDeploys > 0 {
		result.RecommendedWindows = append(result.RecommendedWindows, DeployWindow{
			Day: "Avoid", HourRange: "Weekend", Score: 5, Reason: fmt.Sprintf("%d weekend deploys - change freeze violation", result.Summary.WeekendDeploys),
		})
	}

	// Score
	if totalDeploys > 0 {
		offRatio := float64(result.Summary.OffHoursDeploys) / float64(totalDeploys)
		weekendRatio := float64(result.Summary.WeekendDeploys) / float64(totalDeploys)
		result.OptimizerScore = int((1 - offRatio*0.4 - weekendRatio*0.3) * 100)
		if result.OptimizerScore < 0 {
			result.OptimizerScore = 0
		}
	} else {
		result.OptimizerScore = 100
	}

	// Avg deploys per day (last 30 days)
	result.Summary.AvgDeploysPerDay = float64(totalDeploys) / 30.0

	gradeFromScore(&result.Grade, result.OptimizerScore)

	result.Recommendations = buildDeployWindowRecs(&result)
	writeJSON(w, result)

	// keep import
	_ = sort.Ints
}

func buildDeployWindowRecs(r *DeployWindowOptimizerResult) []string {
	recs := []string{
		fmt.Sprintf("部署窗口: %d 次部署, 峰值 %d:00 (%d 次), 非工作时间 %d 次", r.Summary.TotalDeploys, r.Summary.PeakHour, r.Summary.PeakHourCount, r.Summary.OffHoursDeploys),
	}
	if r.Summary.OffHoursDeploys > 0 {
		recs = append(recs, fmt.Sprintf("警告: %d 次非工作时间部署 (22:00-06:00)", r.Summary.OffHoursDeploys))
	}
	if !r.Summary.ChangeFreezeOK {
		recs = append(recs, fmt.Sprintf("%d 次周末部署 - 违反变更冻结策略", r.Summary.WeekendDeploys))
	}
	recs = append(recs, "推荐窗口: 周二-周四 10:00-14:00 (团队在线, 流量低)")
	if r.OptimizerScore < 60 {
		recs = append(recs, "建议: 实施 change freeze 策略, 禁止周末和非工作时间部署")
	}
	return recs
}
