package appdatabase

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"math/big"

	d_common "github.com/status-im/status-go/common"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"

	"github.com/status-im/status-go/appdatabase/migrations"
	migrationsprevnodecfg "github.com/status-im/status-go/appdatabase/migrationsprevnodecfg"
	"github.com/status-im/status-go/nodecfg"
	"github.com/status-im/status-go/services/wallet/bigint"
	w_common "github.com/status-im/status-go/services/wallet/common"
	"github.com/status-im/status-go/sqlite"

	e_types "github.com/status-im/status-go/eth-node/types"
)

const nodeCfgMigrationDate = 1640111208

var customSteps = []*sqlite.PostStep{
	{Version: 1674136690, CustomMigration: migrateEnsUsernames},
	{Version: 1686048341, CustomMigration: migrateWalletJSONBlobs, RollBackVersion: 1686041510},
	{Version: 1687193315, CustomMigration: migrateWalletTransferFromToAddresses, RollBackVersion: 1686825075},
}

var CurrentAppDBKeyUID string

type DbInitializer struct {
}

func (a DbInitializer) Initialize(path, password string, kdfIterationsNumber int) (*sql.DB, error) {
	return InitializeDB(path, password, kdfIterationsNumber)
}

func doMigration(db *sql.DB) error {
	lastMigration, migrationTableExists, err := sqlite.GetLastMigrationVersion(db)
	if err != nil {
		return err
	}

	if !migrationTableExists || (lastMigration > 0 && lastMigration < nodeCfgMigrationDate) {
		// If it's the first time migration's being run, or latest migration happened before migrating the nodecfg table
		err = migrationsprevnodecfg.Migrate(db)
		if err != nil {
			return err
		}

		// NodeConfig migration cannot be done with SQL
		err = nodecfg.MigrateNodeConfig(db)
		if err != nil {
			return err
		}
	}

	postSteps := []*sqlite.PostStep{
		{Version: 1662365868, CustomMigration: FixMissingKeyUIDForAccounts},
		{Version: 1720606449, CustomMigration: OptimizeMobileWakuV2SettingsForMobileV1},
	}
	postSteps = append(postSteps, customSteps...)
	// Run all the new migrations
	err = migrations.Migrate(db, postSteps)
	if err != nil {
		return err
	}

	return nil
}

// InitializeDB creates db file at a given path and applies migrations.
func InitializeDB(path, password string, kdfIterationsNumber int) (*sql.DB, error) {
	db, err := sqlite.OpenDB(path, password, kdfIterationsNumber)
	if err != nil {
		return nil, err
	}

	err = doMigration(db)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func OptimizeMobileWakuV2SettingsForMobileV1(sqlTx *sql.Tx) error {
	if d_common.IsMobilePlatform() {
		_, err := sqlTx.Exec(`UPDATE wakuv2_config SET light_client = ?, enable_store_confirmation_for_messages_sent = ?`, true, false)
		if err != nil {
			log.Error("failed to enable light client and disable store confirmation for mobile v1", "err", err.Error())
			return err
		}
	}
	return nil
}

func FixMissingKeyUIDForAccounts(sqlTx *sql.Tx) error {
	rows, err := sqlTx.Query(`SELECT address,pubkey FROM accounts WHERE pubkey IS NOT NULL AND type != '' AND type != 'generated'`)
	if err != nil {
		log.Error("Migrating accounts: failed to query accounts", "err", err.Error())
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var address e_types.Address
		var pubkey e_types.HexBytes
		err = rows.Scan(&address, &pubkey)
		if err != nil {
			log.Error("Migrating accounts: failed to scan records", "err", err.Error())
			return err
		}
		pk, err := crypto.UnmarshalPubkey(pubkey)
		if err != nil {
			log.Error("Migrating accounts: failed to unmarshal pubkey", "err", err.Error(), "pubkey", string(pubkey))
			return err
		}
		pkBytes := sha256.Sum256(crypto.FromECDSAPub(pk))
		keyUIDHex := hexutil.Encode(pkBytes[:])
		_, err = sqlTx.Exec(`UPDATE accounts SET key_uid = ? WHERE address = ?`, keyUIDHex, address)
		if err != nil {
			log.Error("Migrating accounts: failed to update key_uid for imported accounts", "err", err.Error())
			return err
		}
	}

	var walletRootAddress e_types.Address
	err = sqlTx.QueryRow(`SELECT wallet_root_address FROM settings WHERE synthetic_id='id'`).Scan(&walletRootAddress)
	if err == sql.ErrNoRows {
		// we shouldn't reach here, but if we do, it probably happened from the test
		log.Warn("Migrating accounts: no wallet_root_address found in settings")
		return nil
	}
	if err != nil {
		log.Error("Migrating accounts: failed to get wallet_root_address", "err", err.Error())
		return err
	}
	_, err = sqlTx.Exec(`UPDATE accounts SET key_uid = ?, derived_from = ? WHERE type = '' OR type = 'generated'`, CurrentAppDBKeyUID, walletRootAddress.Hex())
	if err != nil {
		log.Error("Migrating accounts: failed to update key_uid/derived_from", "err", err.Error())
		return err
	}
	// fix the default wallet account color issue https://github.com/status-im/status-mobile/issues/20476
	// we don't care the other type of account's color
	_, err = sqlTx.Exec(`UPDATE accounts SET color = 'blue',emoji='🐳' WHERE wallet = 1`)
	if err != nil {
		log.Error("Migrating accounts: failed to update default wallet account's color to blue", "err", err.Error())
		return err
	}
	return nil
}

func migrateEnsUsernames(sqlTx *sql.Tx) error {

	// 1. Check if ens_usernames table already exist

	// row := sqlTx.QueryRow("SELECT exists(SELECT name FROM sqlite_master WHERE type='table' AND name='ens_usernames')")
	// tableExists := false
	// err := row.Scan(&tableExists)

	// if err != nil && err != sql.ErrNoRows {
	// 	return err
	// }

	// if tableExists {
	// 	return nil
	// }

	// -- 1. Create new ens_usernames table

	// _, err = sqlTx.Exec(`CREATE TABLE IF NOT EXISTS ens_usernames (
	// 	"username" TEXT NOT NULL,
	// 	"chain_id" UNSIGNED BIGINT DEFAULT 1);`)

	// if err != nil {
	// 	log.Error("Migrating ens usernames: failed to create table", "err", err.Error())
	// 	return err
	// }

	// -- 2. Move current `settings.usernames` to the new table
	/*
		INSERT INTO ens_usernames (username)
			SELECT json_each.value FROM settings, json_each(usernames);
	*/

	rows, err := sqlTx.Query(`SELECT usernames FROM settings`)

	if err != nil {
		log.Error("Migrating ens usernames: failed to query 'settings.usernames'", "err", err.Error())
		return err
	}

	defer rows.Close()

	var usernames []string

	for rows.Next() {
		var usernamesJSON sql.NullString
		err := rows.Scan(&usernamesJSON)

		if err != nil {
			return err
		}

		if !usernamesJSON.Valid {
			continue
		}

		var list []string
		err = json.Unmarshal([]byte(usernamesJSON.String), &list)
		if err != nil {
			return err
		}

		usernames = append(usernames, list...)
	}

	defaultChainID := 1

	for _, username := range usernames {

		var usernameAlreadyMigrated bool

		row := sqlTx.QueryRow(`SELECT EXISTS(SELECT 1 FROM ens_usernames WHERE username=? AND chain_id=?)`, username, defaultChainID)
		err := row.Scan(&usernameAlreadyMigrated)

		if err != nil {
			return err
		}

		if usernameAlreadyMigrated {
			continue
		}

		_, err = sqlTx.Exec(`INSERT INTO ens_usernames (username, chain_id) VALUES (?, ?)`, username, defaultChainID)
		if err != nil {
			log.Error("Migrating ens usernames: failed to insert username into new database", "ensUsername", username, "err", err.Error())
		}
	}

	return nil
}

func MigrateV3ToV4(v3Path string, v4Path string, password string, kdfIterationsNumber int, onStart func(), onEnd func()) error {
	return sqlite.MigrateV3ToV4(v3Path, v4Path, password, kdfIterationsNumber, onStart, onEnd)
}

const (
	batchSize = 1000
)

func migrateWalletJSONBlobs(sqlTx *sql.Tx) error {
	var batchEntries [][]interface{}

	// Extract useful information from the receipt blob and store it as sql interpretable
	//
	// Added tx_hash because the hash column in the transfers table is not (always) the transaction hash.
	// Each entry in that table could either be: A native token (ETH) transfer or ERC20/ERC721 token transfer
	// Added block_hash because the block_hash we have is generated by us and used as block entry ID
	// Added receipt_type, the type we have only indicates if chain or token
	// Added log_index that the log data represents
	//
	// Dropped storing postState because it was replaced by the status after EIP 658
	// Dropped duplicating logs until we have a more structured way to store them.
	// They can be extracted from the transfers.receipt still
	// Dropped the bloom filter because in SQLite is not possible to use it in an
	// efficient manner
	//
	// Extract useful information from the tx blob
	//
	// Added tx_type, which might be different than the receipt type
	//
	// Dropped access_list, need a separate table for it
	// Already there chain_id
	// Dropped v, r, s because I see no way to be useful as BLOBs
	// Added BIGINT values as clamped 64 INT because we can't use 128 bits blobs/strings for int arithmetics
	// _clamped64 prefix indicate clamped 64 bits INT values might be useful for queries (sorting, filtering ...)
	// The amount is stored as a fixed length 128 bit hex string, in
	// order to be able to sort and filter by it
	newColumnsAndIndexSetup := `
		ALTER TABLE transfers ADD COLUMN status INT;
		ALTER TABLE transfers ADD COLUMN receipt_type INT;
		ALTER TABLE transfers ADD COLUMN tx_hash BLOB;
		ALTER TABLE transfers ADD COLUMN log_index INT;
		ALTER TABLE transfers ADD COLUMN block_hash BLOB;
		ALTER TABLE transfers ADD COLUMN cumulative_gas_used INT;
		ALTER TABLE transfers ADD COLUMN contract_address TEXT;
		ALTER TABLE transfers ADD COLUMN gas_used INT;
		ALTER TABLE transfers ADD COLUMN tx_index INT;

		ALTER TABLE transfers ADD COLUMN tx_type INT;
		ALTER TABLE transfers ADD COLUMN protected BOOLEAN;
		ALTER TABLE transfers ADD COLUMN gas_limit UNSIGNED INT;
		ALTER TABLE transfers ADD COLUMN gas_price_clamped64 INT;
		ALTER TABLE transfers ADD COLUMN gas_tip_cap_clamped64 INT;
		ALTER TABLE transfers ADD COLUMN gas_fee_cap_clamped64 INT;
		ALTER TABLE transfers ADD COLUMN amount_padded128hex CHAR(32);
		ALTER TABLE transfers ADD COLUMN account_nonce INT;
		ALTER TABLE transfers ADD COLUMN size INT;
		ALTER TABLE transfers ADD COLUMN token_address BLOB;
		ALTER TABLE transfers ADD COLUMN token_id BLOB;

		CREATE INDEX idx_transfers_filter ON transfers (status, token_address, token_id);`

	rowIndex := 0
	mightHaveRows := true

	_, err := sqlTx.Exec(newColumnsAndIndexSetup)
	if err != nil {
		return err
	}

	for mightHaveRows {
		var chainID uint64
		var hash common.Hash
		var address common.Address
		var entryType string

		rows, err := sqlTx.Query(`SELECT hash, address, network_id, tx, receipt, log, type FROM transfers WHERE tx IS NOT NULL OR receipt IS NOT NULL LIMIT ? OFFSET ?`, batchSize, rowIndex)
		if err != nil {
			return err
		}

		curProcessed := 0
		for rows.Next() {
			tx := &types.Transaction{}
			r := &types.Receipt{}
			l := &types.Log{}

			// Scan row data into the transaction and receipt objects
			nullableTx := sqlite.JSONBlob{Data: tx}
			nullableR := sqlite.JSONBlob{Data: r}
			nullableL := sqlite.JSONBlob{Data: l}
			err = rows.Scan(&hash, &address, &chainID, &nullableTx, &nullableR, &nullableL, &entryType)
			if err != nil {
				rows.Close()
				return err
			}
			var logIndex *uint
			if nullableL.Valid {
				logIndex = new(uint)
				*logIndex = l.Index
			}

			var currentRow []interface{}

			// Check if the receipt is not null before transferring the receipt data
			if nullableR.Valid {
				currentRow = append(currentRow, r.Status, r.Type, r.TxHash, logIndex, r.BlockHash, r.CumulativeGasUsed, r.ContractAddress, r.GasUsed, r.TransactionIndex)
			} else {
				for i := 0; i < 9; i++ {
					currentRow = append(currentRow, nil)
				}
			}

			if nullableTx.Valid {
				correctType, tokenID, value, tokenAddress := extractToken(entryType, tx, l, nullableL.Valid)

				gasPrice := sqlite.BigIntToClampedInt64(tx.GasPrice())
				gasTipCap := sqlite.BigIntToClampedInt64(tx.GasTipCap())
				gasFeeCap := sqlite.BigIntToClampedInt64(tx.GasFeeCap())
				valueStr := sqlite.BigIntToPadded128BitsStr(value)

				currentRow = append(currentRow, tx.Type(), tx.Protected(), tx.Gas(), gasPrice, gasTipCap, gasFeeCap, valueStr, tx.Nonce(), int64(tx.Size()), tokenAddress, (*bigint.SQLBigIntBytes)(tokenID), correctType)
			} else {
				for i := 0; i < 11; i++ {
					currentRow = append(currentRow, nil)
				}
				currentRow = append(currentRow, w_common.EthTransfer)
			}
			currentRow = append(currentRow, hash, address, chainID)
			batchEntries = append(batchEntries, currentRow)

			curProcessed++
		}
		rowIndex += curProcessed

		// Check if there was an error in the last rows.Next()
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		mightHaveRows = (curProcessed == batchSize)

		// insert extracted data into the new columns
		if len(batchEntries) > 0 {
			var stmt *sql.Stmt
			stmt, err = sqlTx.Prepare(`UPDATE transfers SET status = ?, receipt_type = ?, tx_hash = ?, log_index = ?, block_hash = ?, cumulative_gas_used = ?, contract_address = ?, gas_used = ?, tx_index = ?,
				tx_type = ?, protected = ?, gas_limit = ?, gas_price_clamped64 = ?, gas_tip_cap_clamped64 = ?, gas_fee_cap_clamped64 = ?, amount_padded128hex = ?, account_nonce = ?, size = ?, token_address = ?, token_id = ?, type = ?
				WHERE hash = ? AND address = ? AND network_id = ?`)
			if err != nil {
				return err
			}

			for _, dataEntry := range batchEntries {
				_, err = stmt.Exec(dataEntry...)
				if err != nil {
					return err
				}
			}

			// Reset placeHolders and batchEntries for the next batch
			batchEntries = [][]interface{}{}
		}
	}

	return nil
}

func extractToken(entryType string, tx *types.Transaction, l *types.Log, logValid bool) (correctType w_common.Type, tokenID *big.Int, value *big.Int, tokenAddress *common.Address) {
	if logValid {
		correctType, tokenAddress, _, _ = w_common.ExtractTokenTransferData(w_common.Type(entryType), l, tx)
		_, _, _, tokenIDs, values, _ := w_common.ParseTransferLog(*l)
		if len(tokenIDs) > 0 {
			tokenID = tokenIDs[0]
		}
		if len(values) > 0 {
			value = values[0]
		}
	} else {
		correctType = w_common.Type(entryType)
		value = new(big.Int).Set(tx.Value())
	}
	return
}

func migrateWalletTransferFromToAddresses(sqlTx *sql.Tx) error {
	var batchEntries [][]interface{}

	// Extract transfer from/to addresses and add the information into the new columns
	// Re-extract token address and insert it as blob instead of string
	newColumnsAndIndexSetup := `
		ALTER TABLE transfers ADD COLUMN tx_from_address BLOB;
		ALTER TABLE transfers ADD COLUMN tx_to_address BLOB;`

	rowIndex := 0
	mightHaveRows := true

	_, err := sqlTx.Exec(newColumnsAndIndexSetup)
	if err != nil {
		return err
	}

	for mightHaveRows {
		var chainID uint64
		var hash common.Hash
		var address common.Address
		var sender common.Address
		var entryType string

		rows, err := sqlTx.Query(`SELECT hash, address, sender, network_id, tx, log, type FROM transfers WHERE tx IS NOT NULL OR receipt IS NOT NULL LIMIT ? OFFSET ?`, batchSize, rowIndex)
		if err != nil {
			return err
		}

		curProcessed := 0
		for rows.Next() {
			tx := &types.Transaction{}
			l := &types.Log{}

			// Scan row data into the transaction and receipt objects
			nullableTx := sqlite.JSONBlob{Data: tx}
			nullableL := sqlite.JSONBlob{Data: l}
			err = rows.Scan(&hash, &address, &sender, &chainID, &nullableTx, &nullableL, &entryType)
			if err != nil {
				rows.Close()
				return err
			}

			var currentRow []interface{}

			var tokenAddress *common.Address
			var txFrom *common.Address
			var txTo *common.Address

			if nullableTx.Valid {
				if nullableL.Valid {
					_, tokenAddress, txFrom, txTo = w_common.ExtractTokenTransferData(w_common.Type(entryType), l, tx)
				} else {
					txFrom = &sender
					txTo = tx.To()
				}
			}

			currentRow = append(currentRow, tokenAddress, txFrom, txTo)

			currentRow = append(currentRow, hash, address, chainID)
			batchEntries = append(batchEntries, currentRow)

			curProcessed++
		}
		rowIndex += curProcessed

		// Check if there was an error in the last rows.Next()
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		mightHaveRows = (curProcessed == batchSize)

		// insert extracted data into the new columns
		if len(batchEntries) > 0 {
			var stmt *sql.Stmt
			stmt, err = sqlTx.Prepare(`UPDATE transfers SET token_address = ?, tx_from_address = ?, tx_to_address = ?
				WHERE hash = ? AND address = ? AND network_id = ?`)
			if err != nil {
				return err
			}

			for _, dataEntry := range batchEntries {
				_, err = stmt.Exec(dataEntry...)
				if err != nil {
					return err
				}
			}

			// Reset placeHolders and batchEntries for the next batch
			batchEntries = [][]interface{}{}
		}
	}

	return nil
}
