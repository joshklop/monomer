package peptide

import (
	"context"
	"encoding/json"
	"time"

	tmdb "github.com/cometbft/cometbft-db"
	abci "github.com/cometbft/cometbft/abci/types"
	tmlog "github.com/cometbft/cometbft/libs/log"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	tmtypes "github.com/cometbft/cometbft/types"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/joshklop/gm-monomer/app"
	"github.com/joshklop/gm-monomer/app/params"
	"github.com/samber/lo"
)

type Application interface {
	abci.Application
	RollbackToHeight(uint64) error
}

// PeptideApp extends the ABCI-compatible App with additional op-stack L2 chain features
type PeptideApp struct {
	// App is the ABCI-compatible App
	// TODO: IMPORT YOUR ABCI APP HERE
	App Application

	ValSet               *tmtypes.ValidatorSet
	EncodingConfig       *params.EncodingConfig
	lastHeader           *tmproto.Header
	currentHeader        *tmproto.Header
	ChainId              string
	BondDenom            string
	VotingPowerReduction sdk.Int
}

func setPrefixes(accountAddressPrefix string) {
	// Set prefixes
	accountPubKeyPrefix := accountAddressPrefix + "pub"
	validatorAddressPrefix := accountAddressPrefix + "valoper"
	validatorPubKeyPrefix := accountAddressPrefix + "valoperpub"
	consNodeAddressPrefix := accountAddressPrefix + "valcons"
	consNodePubKeyPrefix := accountAddressPrefix + "valconspub"

	// Set and seal config
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount(accountAddressPrefix, accountPubKeyPrefix)
	config.SetBech32PrefixForValidator(validatorAddressPrefix, validatorPubKeyPrefix)
	config.SetBech32PrefixForConsensusNode(consNodeAddressPrefix, consNodePubKeyPrefix)
	config.Seal()
}

func init() {
	// Set prefixes
	setPrefixes(app.AccountAddressPrefix)
}

var DefaultConsensusParams = &tmproto.ConsensusParams{
	Block: &tmproto.BlockParams{
		MaxBytes: 200000,
		MaxGas:   200000000,
	},
	Evidence: &tmproto.EvidenceParams{
		MaxAgeNumBlocks: 302400,
		MaxAgeDuration:  504 * time.Hour, // 3 weeks is the max duration
		MaxBytes:        10000,
	},
	Validator: &tmproto.ValidatorParams{
		PubKeyTypes: []string{
			tmtypes.ABCIPubKeyTypeEd25519,
		},
	},
}

type PeptideAppOptions struct {
	DB                  tmdb.DB
	HomePath            string
	ChainID             string
	IAVLDisableFastNode bool
	IAVLLazyLoading     bool
}

func New(chainID string, dir string, db tmdb.DB, app Application, logger tmlog.Logger) *PeptideApp {
	return NewWithOptions(PeptideAppOptions{
		ChainID:  chainID,
		HomePath: dir,
		DB:       db,
	}, app, logger)
}

// New creates application instance with in-memory database and disabled logging.
func NewWithOptions(options PeptideAppOptions, app Application, logger tmlog.Logger) *PeptideApp {
	logger.Info("new app with options",
		"chain_id", options.ChainID,
		"home_path", options.HomePath,
		"iavl_lazy_loading", options.IAVLLazyLoading,
		"iavl_disable_fast_node", options.IAVLDisableFastNode,
	)

	newPeptideApp := &PeptideApp{
		App:                  app,
		ValSet:               &tmtypes.ValidatorSet{},
		ChainId:              options.ChainID,
		BondDenom:            sdk.DefaultBondDenom,
		VotingPowerReduction: sdk.DefaultPowerReduction,
	}

	return newPeptideApp
}

// ImportAppStateAndValidators imports the application state, init height, and validators from ExportedApp defined by
// the chain App
func (a *PeptideApp) ImportAppStateAndValidators(
	exported *servertypes.ExportedApp,
) abci.ResponseInitChain {
	resp := a.InitChainWithGenesisStateAndHeight(exported.AppState, exported.Height)
	// iterate over exported.Validators
	tmValidators := make([]*tmtypes.Validator, len(exported.Validators))
	for i, val := range exported.Validators {
		tmValidators[i] = tmtypes.NewValidator(val.PubKey, val.Power)
	}
	a.ValSet = tmtypes.NewValidatorSet(tmValidators)
	// set consensusParam?
	return resp
}

func (a *PeptideApp) InitChainWithGenesisState(state app.GenesisState) abci.ResponseInitChain {
	stateBytes := lo.Must(json.MarshalIndent(state, "", " "))
	req := &abci.RequestInitChain{
		ChainId:         a.ChainId,
		ConsensusParams: DefaultConsensusParams,
		Time:            time.Now(),
		AppStateBytes:   stateBytes,
	}
	// InitChain updates deliverState which is required when app.NewContext is called
	return a.App.InitChain(*req)
}

func (a *PeptideApp) InitChainWithGenesisStateAndHeight(state []byte, height int64) abci.ResponseInitChain {
	req := &abci.RequestInitChain{
		ChainId:         a.ChainId,
		ConsensusParams: DefaultConsensusParams,
		AppStateBytes:   state,
		Time:            time.Now(),
		InitialHeight:   height,
	}
	return a.App.InitChain(*req)
}

// This is what initiates the chain app initialisation. It's only meant to be called when the genesis is
// being sealed so the genesis block can be produced.
// - It triggers a call into the base app's InitChain()
// - Commits the app state to disk so it can be persisted across executions
// - Returns a "genesis header" with the genesis block height and app state hash
func (a *PeptideApp) Init(appState []byte, initialHeight int64, genesisTime time.Time) *tmproto.Header {
	response := a.App.InitChain(abci.RequestInitChain{
		ChainId:         a.ChainId,
		ConsensusParams: DefaultConsensusParams,
		AppStateBytes:   appState,
		Time:            genesisTime,
		InitialHeight:   initialHeight,
	})

	// this will store the app state into disk. Failing to call this will result in missing data the next
	// time the app is called
	a.App.Commit()

	// use LastBlockHeight() since it might not be the same as InitialHeight.
	return &tmproto.Header{
		Height:             a.App.Info(abci.RequestInfo{}).LastBlockHeight,
		ValidatorsHash:     a.ValSet.Hash(),
		NextValidatorsHash: a.ValSet.Hash(),
		ChainID:            a.ChainId,
		Time:               genesisTime,
		AppHash:            response.AppHash,
	}
}

// Resume the normal activity after a (chain) restart. It sets the required pointers according to the
// last known header (that comes from the block store) and calls into the base app's BeginBlock()
func (a *PeptideApp) Resume(lastHeader *tmproto.Header, genesisState []byte) error {
	a.lastHeader = lastHeader
	a.currentHeader = &tmproto.Header{
		Height:             a.App.Info(abci.RequestInfo{}).LastBlockHeight + 1,
		ValidatorsHash:     a.ValSet.Hash(),
		NextValidatorsHash: a.ValSet.Hash(),
		ChainID:            a.ChainId,
	}

	a.App.BeginBlock(abci.RequestBeginBlock{Header: *a.CurrentHeader()})
	return nil
}

// Rolls back the app state (i.e. commit multi store from the base app) to the specified height (version)
// If successful, the latest committed version is that of "height"
func (a *PeptideApp) RollbackToHeight(height int64) error {
	return a.App.RollbackToHeight(uint64(height))
}

// Commit pending changes to chain state and start a new block.
// Will error if there is no deliverState, eg. InitChain is not called before first block.
func (a *PeptideApp) CommitAndBeginNextBlock(timestamp eth.Uint64Quantity) *PeptideApp {
	a.App.Commit()
	a.OnCommit(timestamp)

	a.App.BeginBlock(abci.RequestBeginBlock{Header: *a.CurrentHeader()})
	return a
}

// OnCommit updates the last header and current header after App Commit or InitChain
func (a *PeptideApp) OnCommit(timestamp eth.Uint64Quantity) {
	// update last header to the committed time and app hash
	lastHeader := a.currentHeader
	lastHeader.Time = time.Unix(int64(timestamp), 0)
	info := a.App.Info(abci.RequestInfo{})
	lastHeader.AppHash = info.LastBlockAppHash
	a.lastHeader = lastHeader

	// start a new partial header for next round
	a.currentHeader = &tmproto.Header{
		Height:             info.LastBlockHeight + 1,
		ValidatorsHash:     a.ValSet.Hash(),
		NextValidatorsHash: a.ValSet.Hash(),
		ChainID:            a.ChainId,
		Time:               time.Unix(int64(timestamp), 0),
	}
}

// CurrentHeader is the header that is being built, which is not committed yet
func (a *PeptideApp) CurrentHeader() *tmproto.Header {
	return a.currentHeader
}

// LastHeader is the header that was committed, either as a genesis block header or the latest committed block header
func (a *PeptideApp) LastHeader() *tmproto.Header {
	return a.lastHeader
}

// Convert a SDK context to Go context
func (a *PeptideApp) WrapSDKContext(ctx sdk.Context) context.Context {
	return sdk.WrapSDKContext(ctx)
}

// SignMsgs signs a list of Msgs `msg` with `signers`
func (a *PeptideApp) SignMsgs(signers []*SignerAccount, msg ...sdk.Msg) (sdk.Tx, error) {
	tx, err := GenTx(a.EncodingConfig.TxConfig, msg,
		sdk.Coins{sdk.NewInt64Coin(a.BondDenom, 0)},
		DefaultGenTxGas,
		a.ChainId,
		nil,
		signers...,
	)
	return tx, err
}

func (a *PeptideApp) ReportMetrics() {
	// TODO
}
