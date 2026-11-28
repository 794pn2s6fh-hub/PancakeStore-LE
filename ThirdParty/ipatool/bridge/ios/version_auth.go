package main

import (
	"errors"
	"fmt"

	"github.com/majd/ipatool/v2/pkg/appstore"
)

// listVersionsWithNativeRefresh recovers once when Apple rejects either the
// saved commerce token or the Store AMD machine state. It authenticates with
// the credentials stored in the encrypted account keychain, then retries.
// The callbacks keep this policy independently testable from the CGo bridge.
func listVersionsWithNativeRefresh(
	account appstore.Account,
	list func(appstore.Account) (appstore.ListVersionsOutput, error),
	login func(appstore.LoginInput) (appstore.LoginOutput, error),
) (appstore.ListVersionsOutput, error) {
	versions, err := list(account)
	if err == nil || !requiresNativeStoreRefresh(err) {
		return versions, err
	}

	if account.Email == "" || account.Password == "" {
		return appstore.ListVersionsOutput{}, fmt.Errorf(
			"%w; saved Apple credentials are unavailable for automatic refresh; sign in again",
			err,
		)
	}

	fmt.Printf("[GoBridge] %s; refreshing native authentication\n", nativeStoreRefreshReason(err))
	loginResult, loginErr := login(appstore.LoginInput{
		Email:    account.Email,
		Password: account.Password,
	})
	if loginErr != nil {
		return appstore.ListVersionsOutput{}, fmt.Errorf("refresh native Store session: %w", loginErr)
	}

	refreshed := loginResult.Account
	if refreshed.PasswordToken == "" || refreshed.DirectoryServicesID == "" || refreshed.StoreFront == "" {
		return appstore.ListVersionsOutput{}, errors.New("Apple reauthentication returned an incomplete Store account")
	}

	fmt.Println("[GoBridge] native Store authentication refreshed; retrying version lookup")
	return list(refreshed)
}

func requiresNativeStoreRefresh(err error) bool {
	return errors.Is(err, appstore.ErrPasswordTokenExpired) ||
		errors.Is(err, appstore.ErrStoreAMDSynchronizationRequired)
}

func nativeStoreRefreshReason(err error) string {
	if errors.Is(err, appstore.ErrStoreAMDSynchronizationRequired) {
		return "Apple requested AMD machine synchronization"
	}

	return "Apple rejected the saved Store token"
}
