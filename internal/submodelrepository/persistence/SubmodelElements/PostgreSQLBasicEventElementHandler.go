package submodelelements

import (
	"database/sql"
	"errors"
	"time"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLBasicEventElementHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLBasicEventElementHandler(db *sql.DB) (*PostgreSQLBasicEventElementHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLBasicEventElementHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLBasicEventElementHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	basicEvent, ok := submodelElement.(*gen.BasicEventElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type BasicEventElement")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	// BasicEventElement-specific database insertion
	err = insertBasicEventElement(basicEvent, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLBasicEventElementHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	basicEvent, ok := submodelElement.(*gen.BasicEventElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type BasicEventElement")
	}

	// Create the nested basic event element with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// BasicEventElement-specific database insertion for nested element
	err = insertBasicEventElement(basicEvent, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLBasicEventElementHandler) Read(tx *sql.Tx, submodelId string, idShortOrPath string) (gen.SubmodelElement, error) {
	// First, get the base submodel element
	var baseSME gen.SubmodelElement
	id, err := p.decorated.Read(tx, submodelId, idShortOrPath, &baseSME)
	if err != nil {
		return nil, err
	}

	// Check if it's a basic event element
	basicEvent, ok := baseSME.(*gen.BasicEventElement)
	if !ok {
		return nil, errors.New("submodelElement is not of type BasicEventElement")
	}

	// Query basic_event_element
	var observedRef, messageBrokerRef sql.NullInt64
	var direction, state string
	var messageTopic sql.NullString
	var lastUpdate sql.NullTime
	var minInterval, maxInterval sql.NullString

	err = tx.QueryRow(`SELECT observed_ref, direction, state, message_topic, message_broker_ref, last_update, min_interval, max_interval FROM basic_event_element WHERE id = $1`, id).Scan(
		&observedRef, &direction, &state, &messageTopic, &messageBrokerRef, &lastUpdate, &minInterval, &maxInterval)
	if err != nil {
		return nil, err
	}

	basicEvent.Direction = gen.Direction(direction)
	basicEvent.State = gen.StateOfEvent(state)
	if messageTopic.Valid {
		basicEvent.MessageTopic = messageTopic.String
	}
	if lastUpdate.Valid {
		basicEvent.LastUpdate = lastUpdate.Time.Format(time.RFC3339)
	}
	if minInterval.Valid {
		// Assuming minInterval is a string representation of interval
		basicEvent.MinInterval = minInterval.String
	}
	if maxInterval.Valid {
		basicEvent.MaxInterval = maxInterval.String
	}

	if observedRef.Valid {
		ref, err := readReference(tx, observedRef.Int64)
		if err != nil {
			return nil, err
		}
		basicEvent.Observed = ref
	}

	if messageBrokerRef.Valid {
		ref, err := readReference(tx, messageBrokerRef.Int64)
		if err != nil {
			return nil, err
		}
		basicEvent.MessageBroker = ref
	}

	return basicEvent, nil
}
func (p PostgreSQLBasicEventElementHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLBasicEventElementHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

func insertBasicEventElement(basicEvent *gen.BasicEventElement, tx *sql.Tx, id int) error {
	var observedRefID sql.NullInt64
	if !isEmptyReference(basicEvent.Observed) {
		var refID int
		err := tx.QueryRow(`INSERT INTO reference (type) VALUES ($1) RETURNING id`, basicEvent.Observed.Type).Scan(&refID)
		if err != nil {
			return err
		}
		observedRefID = sql.NullInt64{Int64: int64(refID), Valid: true}

		keys := basicEvent.Observed.Keys
		for i := range keys {
			_, err = tx.Exec(`INSERT INTO reference_key (reference_id, position, type, value) VALUES ($1, $2, $3, $4)`,
				refID, i, keys[i].Type, keys[i].Value)
			if err != nil {
				return err
			}
		}
	}

	var messageBrokerRefID sql.NullInt64
	if !isEmptyReference(basicEvent.MessageBroker) {
		var refID int
		err := tx.QueryRow(`INSERT INTO reference (type) VALUES ($1) RETURNING id`, basicEvent.MessageBroker.Type).Scan(&refID)
		if err != nil {
			return err
		}
		messageBrokerRefID = sql.NullInt64{Int64: int64(refID), Valid: true}

		keys := basicEvent.MessageBroker.Keys
		for i := range keys {
			_, err = tx.Exec(`INSERT INTO reference_key (reference_id, position, type, value) VALUES ($1, $2, $3, $4)`,
				refID, i, keys[i].Type, keys[i].Value)
			if err != nil {
				return err
			}
		}
	}

	// Handle nullable fields
	var lastUpdate sql.NullString
	if basicEvent.LastUpdate != "" {
		lastUpdate = sql.NullString{String: basicEvent.LastUpdate, Valid: true}
	}

	var minInterval sql.NullString
	if basicEvent.MinInterval != "" {
		minInterval = sql.NullString{String: basicEvent.MinInterval, Valid: true}
	}

	var maxInterval sql.NullString
	if basicEvent.MaxInterval != "" {
		maxInterval = sql.NullString{String: basicEvent.MaxInterval, Valid: true}
	}

	var messageTopic sql.NullString
	if basicEvent.MessageTopic != "" {
		messageTopic = sql.NullString{String: basicEvent.MessageTopic, Valid: true}
	}

	_, err := tx.Exec(`INSERT INTO basic_event_element (id, observed_ref, direction, state, message_topic, message_broker_ref, last_update, min_interval, max_interval) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		id, observedRefID, basicEvent.Direction, basicEvent.State, messageTopic, messageBrokerRefID, lastUpdate, minInterval, maxInterval)
	return err
}
