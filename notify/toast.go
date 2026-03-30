package notify

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// SendToast sends a Windows 10/11 toast notification via PowerShell
// using System.Windows.Forms.NotifyIcon (balloon tip).
func SendToast(title, message string) error {
	// Escape single quotes for PowerShell
	title = strings.ReplaceAll(title, "'", "''")
	message = strings.ReplaceAll(message, "'", "''")

	script := fmt.Sprintf(`
		Add-Type -AssemblyName System.Windows.Forms
		$notify = New-Object System.Windows.Forms.NotifyIcon
		$notify.Icon = [System.Drawing.SystemIcons]::Information
		$notify.Visible = $true
		$notify.ShowBalloonTip(5000, '%s', '%s', [System.Windows.Forms.ToolTipIcon]::Info)
		Start-Sleep -Milliseconds 100
	`, title, message)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	return cmd.Run()
}

// SendToastModern tries BurntToast module first for richer notifications,
// falling back to the basic NotifyIcon approach if unavailable.
func SendToastModern(title, message string) error {
	title = strings.ReplaceAll(title, "'", "''")
	message = strings.ReplaceAll(message, "'", "''")

	script := fmt.Sprintf(`
		if (Get-Module -ListAvailable -Name BurntToast) {
			New-BurntToastNotification -Text '%s', '%s'
		} else {
			Add-Type -AssemblyName System.Windows.Forms
			$n = New-Object System.Windows.Forms.NotifyIcon
			$n.Icon = [System.Drawing.SystemIcons]::Information
			$n.Visible = $true
			$n.ShowBalloonTip(5000, '%s', '%s', 'Info')
			Start-Sleep -Milliseconds 100
		}
	`, title, message, title, message)

	return exec.Command("powershell", "-NoProfile", "-Command", script).Run()
}

// Send fires a toast notification asynchronously. Errors are logged but not returned.
func Send(title, message string) {
	go func() {
		if err := SendToast(title, message); err != nil {
			log.Printf("[Notify] Toast failed: %v", err)
		}
	}()
}
