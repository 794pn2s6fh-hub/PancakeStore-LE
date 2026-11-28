package main

import (
	"errors"
	"fmt"

	"github.com/majd/ipatool/v2/pkg/appstore"
)

// downloadWithNativeRefresh refreshes a rejected Store token or AMD machine
// session once, then retries the exact same version download.
func downloadWithNativeRefresh(
	account appstore.Account,
	download func(appstore.Account) (appstore.DownloadOutput, error),
	login func(appstore.LoginInput) (appstore.LoginOutput, error),
) (appstore.DownloadOutput, appstore.Account, error) {
	output, err := download(account)
	if err == nil || !requiresNativeStoreRefresh(err) {
		return output, account, err
	}

	refreshed, err := refreshNativeAccount(account, login, err)
	if err != nil {
		return appstore.DownloadOutput{}, account, err
	}

	fmt.Println("[GoBridge] native Store authentication refreshed; retrying IPA download")
	output, err = download(refreshed)
	return output, refreshed, err
}

// purchaseWithNativeRefresh is used only after Apple says a free app has no
// license. Paid apps are never purchased automatically by PancakeStore.
func purchaseWithNativeRefresh(
	account appstore.Account,
	purchase func(appstore.Account) error,
	login func(appstore.LoginInput) (appstore.LoginOutput, error),
) (appstore.Account, error) {
	err := purchase(account)
	if err == nil || errors.Is(err, appstore.ErrLicenseAlreadyExists) {
		return account, nil
	}
	if !requiresNativeStoreRefresh(err) {
		return account, err
	}

	refreshed, refreshErr := refreshNativeAccount(account, login, err)
	if refreshErr != nil {
		return account, refreshErr
	}

	fmt.Println("[GoBridge] retrying free-app license acquisition with refreshed Store session")
	err = purchase(refreshed)
	if errors.Is(err, appstore.ErrLicenseAlreadyExists) {
		err = nil
	}
	return refreshed, err
}

func refreshNativeAccount(
	account appstore.Account,
	login func(appstore.LoginInput) (appstore.LoginOutput, error),
	trigger error,
) (appstore.Account, error) {
	if account.Email == "" || account.Password == "" {
		return appstore.Account{}, errors.New("saved Apple credentials are unavailable; sign in again")
	}

	fmt.Printf("[GoBridge] %s; refreshing native authentication\n", nativeStoreRefreshReason(trigger))
	loginResult, err := login(appstore.LoginInput{
		Email:    account.Email,
		Password: account.Password,
	})
	if err != nil {
		return appstore.Account{}, fmt.Errorf("refresh native Store session: %w", err)
	}

	refreshed := loginResult.Account
	if refreshed.PasswordToken == "" || refreshed.DirectoryServicesID == "" || refreshed.StoreFront == "" {
		return appstore.Account{}, errors.New("Apple reauthentication returned an incomplete Store account")
	}

	return refreshed, nil
}
