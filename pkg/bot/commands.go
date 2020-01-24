package bot

import (
	"fmt"
	"gitlab.com/postgres-ai/database-lab/pkg/log"
	"gitlab.com/postgres-ai/joe/pkg/chatapi"
	"gitlab.com/postgres-ai/joe/pkg/pgexplain"
	"gitlab.com/postgres-ai/joe/pkg/util"
	"time"
)

func Explain(b *Bot, query string, apiCmd *ApiCommand, msg *chatapi.Message, ch, connStr string) {
	var detailsText string
	var trnd bool

	explainConfig := b.Config.Explain

	if query == "" {
		failMsg(msg, MSG_EXPLAIN_OPTION_REQ)
		b.FailApiCmd(apiCmd, MSG_EXPLAIN_OPTION_REQ)
		return
	}

	// Explain request and show.
	var res, err = dbExplain(connStr, query)
	if err != nil {
		failMsg(msg, err.Error())
		b.FailApiCmd(apiCmd, err.Error())
		return
	}

	apiCmd.PlanText = res
	planPreview, trnd := cutText(res, PLAN_SIZE, SEPARATOR_PLAN)

	msgInitText := msg.Text

	err = msg.Append(fmt.Sprintf("*Plan:*\n```%s```", planPreview))
	if err != nil {
		log.Err("Show plan: ", err)
		failMsg(msg, err.Error())
		b.FailApiCmd(apiCmd, err.Error())
		return
	}

	filePlanWoExec, err := b.Chat.UploadFile("plan-wo-execution-text", res, ch, msg.Timestamp)
	if err != nil {
		log.Err("File upload failed:", err)
		failMsg(msg, err.Error())
		return
	}

	detailsText = ""
	if trnd {
		detailsText = " " + CUT_TEXT
	}

	err = msg.Append(fmt.Sprintf("<%s|Full plan (w/o execution)>%s", filePlanWoExec.Permalink, detailsText))
	if err != nil {
		log.Err("File: ", err)
		failMsg(msg, err.Error())
		b.FailApiCmd(apiCmd, err.Error())
		return
	}

	// Explain analyze request and processing.
	res, err = dbExplainAnalyze(connStr, query)
	if err != nil {
		failMsg(msg, err.Error())
		b.FailApiCmd(apiCmd, err.Error())
		return
	}

	apiCmd.PlanExecJson = res

	// Visualization.
	explain, err := pgexplain.NewExplain(res, explainConfig)
	if err != nil {
		log.Err("Explain parsing: ", err)
		failMsg(msg, err.Error())
		b.FailApiCmd(apiCmd, err.Error())
		return
	}

	vis := explain.RenderPlanText()
	apiCmd.PlanExecText = vis

	planExecPreview, trnd := cutText(vis, PLAN_SIZE, SEPARATOR_PLAN)

	err = msg.Replace(msgInitText + chatapi.CHAT_APPEND_SEPARATOR +
		fmt.Sprintf("*Plan with execution:*\n```%s```", planExecPreview))
	if err != nil {
		log.Err("Show the plan with execution:", err)
		failMsg(msg, err.Error())
		b.FailApiCmd(apiCmd, err.Error())
		return
	}

	_, err = b.Chat.UploadFile("plan-json", res, ch, msg.Timestamp)
	if err != nil {
		log.Err("File upload failed:", err)
		failMsg(msg, err.Error())
		b.FailApiCmd(apiCmd, err.Error())
		return
	}

	filePlan, err := b.Chat.UploadFile("plan-text", vis, ch, msg.Timestamp)
	if err != nil {
		log.Err("File upload failed:", err)
		failMsg(msg, err.Error())
		b.FailApiCmd(apiCmd, err.Error())
		return
	}

	detailsText = ""
	if trnd {
		detailsText = " " + CUT_TEXT
	}

	err = msg.Append(fmt.Sprintf("<%s|Full execution plan>%s \n"+
		"_Other artifacts are provided in the thread_",
		filePlan.Permalink, detailsText))
	if err != nil {
		log.Err("File: ", err)
		failMsg(msg, err.Error())
		b.FailApiCmd(apiCmd, err.Error())
		return
	}

	// Recommendations.
	tips, err := explain.GetTips()
	if err != nil {
		log.Err("Recommendations: ", err)
		failMsg(msg, err.Error())
		b.FailApiCmd(apiCmd, err.Error())
		return
	}

	recommends := ""
	if len(tips) == 0 {
		recommends += ":white_check_mark: Looks good"
	} else {
		for _, tip := range tips {
			recommends += fmt.Sprintf(
				":exclamation: %s – %s <%s|Show details>\n", tip.Name,
				tip.Description, tip.DetailsUrl)
		}
	}

	apiCmd.Recommendations = recommends

	err = msg.Append("*Recommendations:*\n" + recommends)
	if err != nil {
		log.Err("Show recommendations: ", err)
		failMsg(msg, err.Error())
		b.FailApiCmd(apiCmd, err.Error())
		return
	}

	// Summary.
	stats := explain.RenderStats()
	apiCmd.Stats = stats

	err = msg.Append(fmt.Sprintf("*Summary:*\n```%s```", stats))
	if err != nil {
		log.Err("Show summary: ", err)
		failMsg(msg, err.Error())
		b.FailApiCmd(apiCmd, err.Error())
		return
	}
}


func Exec(b *Bot, query string, apiCmd *ApiCommand, msg *chatapi.Message, connStr string){
	if query == "" {
		failMsg(msg, MSG_EXEC_OPTION_REQ)
		b.failApiCmd(apiCmd, MSG_EXEC_OPTION_REQ)
		return
	}

	start := time.Now()
	err := dbExec(connStr, query)
	elapsed := time.Since(start)
	if err != nil {
		log.Err("Exec:", err)
		failMsg(msg, err.Error())
		b.failApiCmd(apiCmd, err.Error())
		return
	}

	duration := util.DurationToString(elapsed)
	result := fmt.Sprintf("The query has been executed. Duration: %s", duration)
	apiCmd.Response = result

	err = msg.Append(result)
	if err != nil {
		log.Err("Exec:", err)
		failMsg(msg, err.Error())
		b.failApiCmd(apiCmd, err.Error())
		return
	}
}
