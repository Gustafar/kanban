package notify

import (
	"os"
	"os/exec"
	"runtime"

	"kanban_board/internal/proc"
)

const script = `
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$t = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$n = $t.GetElementsByTagName('text')
$n.Item(0).AppendChild($t.CreateTextNode($env:KB_TITLE)) | Out-Null
$n.Item(1).AppendChild($t.CreateTextNode($env:KB_BODY)) | Out-Null
$app = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($app).Show([Windows.UI.Notifications.ToastNotification]::new($t))
`

// Toast shows a native Windows notification. Title/body travel through env vars
// so nothing user-controlled is ever interpolated into the script.
func Toast(title, body string) {
	if runtime.GOOS != "windows" {
		return
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	proc.Hide(cmd)
	cmd.Env = append(os.Environ(), "KB_TITLE="+title, "KB_BODY="+body)
	_ = cmd.Run()
}
