package auth

import (
	gitCredentialCmd "github.com/cli/cli/pkg/cmd/auth/gitcredential"
	authLoginCmd "github.com/cli/cli/pkg/cmd/auth/login"
	authLogoutCmd "github.com/cli/cli/pkg/cmd/auth/logout"
	authRefreshCmd "github.com/cli/cli/pkg/cmd/auth/refresh"
	authStatusCmd "github.com/cli/cli/pkg/cmd/auth/status"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)


	cmd := &cobra.Command{
		Use:   "auth <command>",
		Short: "Login, logout, and refresh your authentication",
		Long:  `Manage go's authentication state.`,
	}

	cmdutil.DisableAuthCheck(cmd)

	cmd.AddCommand(authLoginCmd.NewCmdLogin(f, nil))
	cmd.AddCommand(authLogoutCmd.NewCmdLogout(f, nil))
	cmd.AddCommand(authStatusCmd.NewCmdStatus(f, nil))
	cmd.AddCommand(authRefreshCmd.NewCmdRefresh(f, nil))
	cmd.AddCommand(gitCredentialCmd.NewCmdCredential(f, nil))

	return cmd
}
