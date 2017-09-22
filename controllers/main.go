package controllers

import (
	"strconv"
	"strings"
	"time"

	"runtime"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/utils"
	"github.com/dchest/captcha"
	"github.com/go-crontab/jobs"
	"github.com/go-crontab/libs"
	"github.com/go-crontab/models"
)

type MainController struct {
	BaseController
}

//首页
func (this *MainController) Index() {
	this.Data["pageTitle"] = "系统概况"
	// 分组列表
	groups, _ := models.GetTaskGroupList(1, 100)
	groups_map := make(map[int]string)
	for _, gname := range groups {
		groups_map[gname.Id] = gname.GroupName
	}
	//计算总任务数量
	_, count := models.TaskGetList(1, 200)
	// 即将执行的任务
	entries := jobs.GetEntries(30)
	jobList := make([]map[string]interface{}, len(entries))
	startJob := 0 //即将执行的任务
	for k, v := range entries {
		row := make(map[string]interface{})
		job := v.Job.(*jobs.Job)
		task, _ := models.TaskGetById(job.GetId())
		row["task_id"] = job.GetId()
		row["task_name"] = job.GetName()
		row["task_group"] = groups_map[task.GroupId]
		row["next_time"] = beego.Date(v.Next, "Y-m-d H:i:s")
		jobList[k] = row
		startJob++
	}

	// 最近执行的日志
	logs, _ := models.TaskLogGetList(1, 20)
	recentLogs := make([]map[string]interface{}, len(logs))
	failJob := 0 //最近失败的数量
	okJob := 0   //最近成功的数量
	for k, v := range logs {
		task, err := models.TaskGetById(v.TaskId)
		taskName := ""
		if err == nil {
			taskName = task.TaskName
		}
		row := make(map[string]interface{})
		row["task_name"] = taskName
		row["id"] = v.Id
		row["start_time"] = beego.Date(time.Unix(v.CreateTime, 0), "Y-m-d H:i:s")
		row["process_time"] = float64(v.ProcessTime) / 1000
		row["ouput_size"] = libs.SizeFormat(float64(len(v.Output)))
		row["output"] = beego.Substr(v.Output, 0, 100)
		row["status"] = v.Status
		recentLogs[k] = row
		if v.Status != 0 {
			failJob++
		} else {
			okJob++
		}
	}

	// 最近执行失败的日志
	logs, _ = models.TaskLogGetList(1, 20, "status__lt", 0)
	errLogs := make([]map[string]interface{}, len(logs))

	for k, v := range logs {
		task, err := models.TaskGetById(v.TaskId)
		taskName := ""
		if err == nil {
			taskName = task.TaskName
		}

		row := make(map[string]interface{})
		row["task_name"] = taskName
		row["id"] = v.Id
		row["start_time"] = beego.Date(time.Unix(v.CreateTime, 0), "Y-m-d H:i:s")
		row["process_time"] = float64(v.ProcessTime) / 1000
		row["ouput_size"] = libs.SizeFormat(float64(len(v.Output)))
		row["error"] = beego.Substr(v.Error, 0, 100)
		row["status"] = v.Status
		errLogs[k] = row

	}

	this.Data["startJob"] = startJob
	this.Data["okJob"] = okJob
	this.Data["failJob"] = failJob
	this.Data["totalJob"] = count

	this.Data["recentLogs"] = recentLogs
	// this.Data["errLogs"] = errLogs
	this.Data["jobs"] = jobList
	this.Data["cpuNum"] = runtime.NumCPU()
	this.display()
}

//个人信息
func (this *MainController) Profile() {
	beego.ReadFromRequest(&this.Controller)
	user, _ := models.GetUserById(this.userId)
	if this.isPost() {
		user.Email = this.GetString("email")
		user.Update()
		password1 := this.GetString("password1")
		password2 := this.GetString("password2")
		if password1 != "" {
			if len(password1) < 6 {
				this.ajaxMsg("密码长度必须大于6位", MSG_ERR)
			} else if password2 != password1 {
				this.ajaxMsg("两次输入的密码不一致", MSG_ERR)
			} else {
				user.Salt = string(utils.RandomCreateBytes(10))
				user.Password = libs.Md5([]byte(password1 + user.Salt))
				user.Update()
			}
		}
		this.ajaxMsg("", MSG_OK)
	}
	this.Data["pageTitle"] = "资料修改"
	this.Data["user"] = user
	this.display()
}

func (this *MainController) Login() {

	if this.userId > 0 {
		this.redirect("/")
	}
	beego.ReadFromRequest(&this.Controller)
	if this.isPost() {
		flash := beego.NewFlash()
		errmsg := ""

		username := strings.TrimSpace(this.GetString("username"))
		password := strings.TrimSpace(this.GetString("password"))
		captchaValue := strings.TrimSpace(this.GetString("captcha"))
		remember := this.GetString("remember")
		captchaId := this.GetString("captchaId")
		if username != "" && password != "" && captchaValue != "" {

			//验证码校验
			if !captcha.VerifyString(captchaId, captchaValue) {
				errmsg = "验证码错误！"
				flash.Error(errmsg)
				flash.Store(&this.Controller)
				this.redirect(beego.URLFor("MainController.Login"))
			}

			user, err := models.GetUserByName(username)

			if err != nil || user.Password != libs.Md5([]byte(password+user.Salt)) {
				errmsg = "帐号或密码错误"
			} else if user.Status == -1 {
				errmsg = "该帐号已禁用"
			} else {
				user.LastIp = this.getClientIp()
				user.LastLogin = time.Now().Unix()
				models.UpdateUser(user)

				authkey := libs.Md5([]byte(this.getClientIp() + "|" + user.Password + user.Salt))
				if remember == "yes" {
					this.Ctx.SetCookie("auth", strconv.Itoa(user.Id)+"|"+authkey, 7*86400)
				} else {
					this.Ctx.SetCookie("auth", strconv.Itoa(user.Id)+"|"+authkey, 86400)
				}
				this.redirect(beego.URLFor("TaskController.List"))
			}
			flash.Error(errmsg)
			flash.Store(&this.Controller)
			this.redirect(beego.URLFor("MainController.Login"))

		}
	}

	//验证码
	d := struct {
		CaptchaId string
	}{
		captcha.NewLen(4),
	}
	this.Data["CaptchaId"] = d.CaptchaId
	this.TplName = "public/login.html"
}

func (this *MainController) Logout() {
	this.Ctx.SetCookie("auth", "")
	this.redirect(beego.URLFor("MainController.Login"))
}

// 获取系统时间
func (this *MainController) GetTime() {
	out := make(map[string]interface{})
	out["time"] = time.Now().UnixNano() / int64(time.Millisecond)
	this.jsonResult(out)
}
