package testappv1

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (s *SetRequest) ValidateBasic() error {
	return nil
}

func (s *SetRequest) GetSigners() []sdk.AccAddress {
	return nil
}
