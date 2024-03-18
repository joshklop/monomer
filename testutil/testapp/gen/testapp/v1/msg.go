package testappv1

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var _ sdk.Msg = (*SetRequest)(nil)

func (r *SetRequest) GetSigners() []sdk.AccAddress {
	return nil
}

func (r *SetRequest) ValidateBasic() error {
	return nil
}
