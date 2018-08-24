package models

import (
	"fmt"
	"time"

	upnp "github.com/NebulousLabs/go-upnp/goupnp"
	"github.com/jmoiron/sqlx"
)

// LineupDB is a struct containing initialized the SQL connection as well as the APICollection.
type LineupDB struct {
	SQL        *sqlx.DB
	Collection *APICollection
}

func newLineupDB(
	SQL *sqlx.DB,
	Collection *APICollection,
) *LineupDB {
	db := &LineupDB{
		SQL:        SQL,
		Collection: Collection,
	}
	return db
}

func (db *LineupDB) tableName() string {
	return "lineup"
}

// DiscoveryData contains data about telly to expose in the HDHomeRun format for Plex detection.
type DiscoveryData struct {
	FriendlyName    string
	Manufacturer    string
	ModelName       string
	ModelNumber     string
	FirmwareName    string
	TunerCount      int
	FirmwareVersion string
	DeviceID        string
	DeviceAuth      string
	BaseURL         string
	LineupURL       string
	DeviceUUID      string
}

// UPNP returns the UPNP representation of the DiscoveryData.
func (d *DiscoveryData) UPNP() upnp.RootDevice {
	return upnp.RootDevice{
		SpecVersion: upnp.SpecVersion{
			Major: 1, Minor: 0,
		},
		URLBaseStr: d.BaseURL,
		Device: upnp.Device{
			DeviceType:       "urn:schemas-upnp-org:device:MediaServer:1",
			FriendlyName:     d.FriendlyName,
			Manufacturer:     d.Manufacturer,
			ModelName:        d.ModelName,
			ModelNumber:      d.ModelNumber,
			ModelDescription: fmt.Sprintf("%s %s", d.ModelNumber, d.ModelName),
			SerialNumber:     d.DeviceID,
			UDN:              d.DeviceUUID,
			PresentationURL: upnp.URLField{
				Str: "/",
			},
		},
	}
}

type SQLLineup struct {
	ID               int        `db:"id"`
	Name             string     `db:"name"`
	SSDP             bool       `db:"ssdp"`
	ListenAddress    string     `db:"listen_address"`
	DiscoveryAddress string     `db:"discovery_address"`
	Port             int        `db:"port"`
	Tuners           int        `db:"tuners"`
	Manufacturer     string     `db:"manufacturer"`
	ModelName        string     `db:"model_name"`
	ModelNumber      string     `db:"model_number"`
	FirmwareName     string     `db:"firmware_name"`
	FirmwareVersion  string     `db:"firmware_version"`
	DeviceID         string     `db:"device_id"`
	DeviceAuth       string     `db:"device_auth"`
	DeviceUUID       string     `db:"device_uuid"`
	CreatedAt        *time.Time `db:"created_at"`

	Channels []LineupChannel
}

func (s *SQLLineup) GetDiscoveryData() DiscoveryData {
	baseAddr := fmt.Sprintf("http://%s:%d", s.DiscoveryAddress, s.Port)
	return DiscoveryData{
		FriendlyName:    s.Name,
		Manufacturer:    s.Manufacturer,
		ModelName:       s.ModelName,
		ModelNumber:     s.ModelNumber,
		FirmwareName:    s.FirmwareName,
		TunerCount:      s.Tuners,
		FirmwareVersion: s.FirmwareVersion,
		DeviceID:        s.DeviceID,
		DeviceAuth:      s.DeviceAuth,
		BaseURL:         baseAddr,
		LineupURL:       fmt.Sprintf("%s/lineup.json", baseAddr),
		DeviceUUID:      s.DeviceUUID,
	}
}

// LineupAPI contains all methods for the User struct
type LineupAPI interface {
	InsertLineup(lineupStruct SQLLineup) (*SQLLineup, error)
	DeleteLineup(lineupID int) (*SQLLineup, error)
	UpdateLineup(lineupID int, description string) (*SQLLineup, error)
	GetLineupByID(id int, withChannels bool) (*SQLLineup, error)
	GetEnabledLineups(withChannels bool) ([]SQLLineup, error)
}

const baseLineupQuery string = `
SELECT
  L.id,
  L.name,
  L.ssdp,
  L.listen_address,
  L.discovery_address,
  L.port,
  L.tuners,
  L.manufacturer,
  L.model_name,
  L.model_number,
  L.firmware_name,
  L.firmware_version,
  L.device_id,
  L.device_auth,
  L.device_uuid,
  L.created_at
  FROM lineup L`

// InsertLineup inserts a new Lineup into the database.
func (db *LineupDB) InsertLineup(lineupStruct SQLLineup) (*SQLLineup, error) {
	lineup := SQLLineup{}
	res, err := db.SQL.NamedExec(`
    INSERT INTO lineup (name, ssdp, listen_address, discovery_address, port, tuners, manufacturer, model_name, model_number, firmware_name, firmware_version, device_id, device_auth, device_uuid)
    VALUES (:name, :ssdp, :listen_address, :discovery_address, :port, :tuners, :manufacturer, :model_name, :model_number, :firmware_name, :firmware_version, :device_id, :device_auth, :device_uuid)`, lineupStruct)
	if err != nil {
		return &lineup, err
	}
	rowID, rowIDErr := res.LastInsertId()
	if rowIDErr != nil {
		return &lineup, rowIDErr
	}
	err = db.SQL.Get(&lineup, "SELECT * FROM lineup WHERE id = $1", rowID)
	return &lineup, err
}

// GetLineupByID returns a single Lineup for the given ID.
func (db *LineupDB) GetLineupByID(id int, withChannels bool) (*SQLLineup, error) {
	var lineup SQLLineup
	err := db.SQL.Get(&lineup, fmt.Sprintf(`%s WHERE L.id = $1`, baseLineupQuery), id)
	if withChannels {
		channels, channelsErr := db.Collection.LineupChannel.GetChannelsForLineup(lineup.ID, true)
		if channelsErr != nil {
			return nil, channelsErr
		}
		lineup.Channels = channels
	}
	return &lineup, err
}

// DeleteLineup marks a lineup with the given ID as deleted.
func (db *LineupDB) DeleteLineup(lineupID int) (*SQLLineup, error) {
	lineup := SQLLineup{}
	err := db.SQL.Get(&lineup, `DELETE FROM lineup WHERE id = $1`, lineupID)
	return &lineup, err
}

// UpdateLineup updates a lineup.
func (db *LineupDB) UpdateLineup(lineupID int, description string) (*SQLLineup, error) {
	lineup := SQLLineup{}
	err := db.SQL.Get(&lineup, `UPDATE lineup SET description = $2 WHERE id = $1 RETURNING *`, lineupID, description)
	return &lineup, err
}

// GetEnabledLineups returns all enabled lineups in the database.
func (db *LineupDB) GetEnabledLineups(withChannels bool) ([]SQLLineup, error) {
	lineups := make([]SQLLineup, 0)
	err := db.SQL.Select(&lineups, baseLineupQuery)
	if withChannels {
		for idx, lineup := range lineups {
			channels, channelsErr := db.Collection.LineupChannel.GetChannelsForLineup(lineup.ID, true)
			if channelsErr != nil {
				return nil, channelsErr
			}
			lineup.Channels = channels
			lineups[idx] = lineup
		}
	}
	return lineups, err
}
