package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

// LlamaCppTemplate is the sandbox template to use for llama.cpp processes.
const LlamaCppTemplate = `(version 1)

;;; Keep a default allow policy (because encoding things like DYLD support and
;;; device access is quite difficult), but deny critical exploitation targets
;;; (generally aligned with the App Sandbox entitlements that aren't on by
;;; default). In theory we'll be subject to the Docker.app sandbox as well
;;; (unless we're running standalone), but even Docker.app has a very privileged
;;; sandbox, so we need additional constraints.
;;;
;;; Note: The following are known to be required at some level for llama.cpp
;;; (though we could further experiment to deny certain sub-permissions):
;;;   - authorization
;;;   - darwin
;;;   - iokit
;;;   - mach
;;;   - socket
;;;   - syscall
;;;   - process
(allow default)

;;; Deny network access, except for our IPC sockets.
(deny network*)
(allow network-bind network-inbound
    (regex #"inference-runner-[0-9]+\.sock$"))

;;; Deny access to the camera and microphone.
(deny device*)

;;; Deny access to NVRAM settings.
(deny nvram*)

;;; Deny access to system-level privileges.
(deny system*)

;;; Deny access to job creation.
(deny job-creation)

;;; Don't allow new executable code to be created in memory at runtime.
(deny dynamic-code-generation)

;;; Disable access to user preferences.
(deny user-preference*)

;;; Restrict file access.
;;; NOTE: For some reason, the (home-subpath "...") predicate used in system
;;; sandbox profiles doesn't work with sandbox-exec.
;;; NOTE: We have to allow access to the working directory for standalone mode.
;;; NOTE: For some reason (deny file-read*) really doesn't like to play nice
;;; with llama.cpp, so for that reason we'll avoid a blanket ban and just ban
;;; directories that might contain sensitive data.
(deny file-map-executable)
(deny file-write*)
(deny file-read*
    (subpath "/Applications")
    (subpath "/private/etc")
	(subpath "/Library")
	(subpath "/Users")
	(subpath "/Volumes"))
(allow file-read* file-map-executable
    (subpath "/usr")
    (subpath "/System")
    (subpath "/Applications/Docker.app/Contents/Resources/model-runner")
    (subpath "[HOMEDIR]/.docker/bin/inference")
    (subpath "[HOMEDIR]/.docker/bin/lib"))
(allow file-write*
    (regex #"inference-runner-[0-9]+\.sock$")
    (literal "/dev/null")
    (subpath "/private/var")
    (subpath "[WORKDIR]"))
(allow file-read*
    (subpath "[WORKDIR]")
    (subpath "[HOMEDIR]/.docker/models"))
`

// CommandContext creates a sandboxed version of an os/exec.Cmd. On Darwin, we
// use the sandbox-exec command to wrap the process.
func CommandContext(ctx context.Context, template, name string, args ...string) (*exec.Cmd, error) {
	// Look up the user's home directory.
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("unable to lookup user: %w", err)
	}

	// Look up the working directory.
	currentDirectory, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("unable to determine working directory: %w", err)
	}

	// Compute the profile. Switch to text/template if this gets more complex.
	profile := strings.ReplaceAll(template, "[HOMEDIR]", currentUser.HomeDir)
	profile = strings.ReplaceAll(profile, "[WORKDIR]", currentDirectory)

	// Create the sandboxed process.
	sandboxedArgs := make([]string, 0, len(args)+3)
	sandboxedArgs = append(sandboxedArgs, "-p", profile, name)
	sandboxedArgs = append(sandboxedArgs, args...)
	return exec.CommandContext(ctx, "sandbox-exec", sandboxedArgs...), nil
}
