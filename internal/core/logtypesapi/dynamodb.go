package logtypesapi

/**
 * Panther is a Cloud-Native SIEM for the Modern Security Team.
 * Copyright (C) 2020 Panther Labs Inc
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as
 * published by the Free Software Foundation, either version 3 of the
 * License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/panther-labs/panther/internal/core/logtypesapi/transact"
	"github.com/panther-labs/panther/pkg/lambdalogger"
)

var L = lambdalogger.FromContext

const (
	// We will use this kind of record to store custom log types
	// For backwards compatibility the value is 'custom'
	recordKindSchema = "custom"

	attrRecordKind = "RecordKind"
	attrRevision   = "revision"
	attrManaged    = "managed"
)

var _ SchemaDatabase = (*DynamoDBSchemas)(nil)

// DynamoDBSchemas provides logtypes api actions for DDB
type DynamoDBSchemas struct {
	DB        dynamodbiface.DynamoDBAPI
	TableName string
}

func (d *DynamoDBSchemas) ScanSchemas(ctx context.Context, scan ScanSchemaFunc) error {
	filter, err := expression.NewBuilder().WithFilter(
		expression.Name(attrRecordKind).Equal(expression.Value(recordKindSchema)),
	).Build()
	if err != nil {
		return err
	}
	var itemErr error
	scanErr := d.DB.ScanPagesWithContext(ctx, &dynamodb.ScanInput{
		FilterExpression:          filter.Filter(),
		ExpressionAttributeNames:  filter.Names(),
		ExpressionAttributeValues: filter.Values(),
		TableName:                 aws.String(d.TableName),
	}, func(page *dynamodb.ScanOutput, isLast bool) bool {
		for _, item := range page.Items {
			record := SchemaRecord{}
			if itemErr = dynamodbattribute.UnmarshalMap(item, &record); itemErr != nil {
				return false
			}
			if !scan(&record) {
				return false
			}
		}
		return true
	})
	if scanErr != nil {
		return scanErr
	}
	if itemErr != nil {
		return itemErr
	}
	return nil
}

func (d *DynamoDBSchemas) GetSchema(ctx context.Context, id string) (*SchemaRecord, error) {
	input := dynamodb.GetItemInput{
		TableName: aws.String(d.TableName),
		Key:       mustMarshalMap(schemaRecordKey(id)),
	}
	output, err := d.DB.GetItemWithContext(ctx, &input)
	if err != nil {
		return nil, err
	}
	L(ctx).Debug("retrieved schema record",
		zap.String("logType", id),
		zap.Any("item", output.Item))

	record := ddbSchemaRecord{}
	if err := dynamodbattribute.UnmarshalMap(output.Item, &record); err != nil {
		return nil, err
	}
	if record.Name == "" {
		return nil, nil
	}
	return &record.SchemaRecord, nil
}

// nolint:lll
func (d *DynamoDBSchemas) PutSchema(ctx context.Context, id string, record *SchemaRecord) (*SchemaRecord, error) {
	tx := buildPutSchemaTx(d.TableName, id, *record)
	input, err := tx.Build()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to build update managed schema transaction")
	}
	if _, err := d.DB.TransactWriteItemsWithContext(ctx, input); err != nil {
		return nil, errors.Wrap(tx.ExplainTransactionError(err), "update schema transaction failed")
	}
	return record, nil
}

func buildPutSchemaTx(tableName string, id string, record SchemaRecord) transact.Transaction {
	currentRevision := record.Revision - 1
	return transact.Transaction{
		&transact.Update{
			TableName: tableName,
			Key:       schemaRecordKey(id),
			Set: map[string]interface{}{
				transact.SetIfNotExists: struct {
					CreatedAt time.Time `dynamodbav:"createdAt"`
					Name      string    `dynamodbav:"logType"`
					Managed   bool      `dynamodbav:"managed"`
				}{
					CreatedAt: record.CreatedAt,
					Name:      record.Name,
					Managed:   record.Managed,
				},
				transact.SetAll: struct {
					UpdatedAt    time.Time `dynamodbav:"updatedAt"`
					Release      string    `dynamodbav:"release"`
					Revision     int64     `dynamodbav:"revision"`
					Description  string    `dynamodbav:"description"`
					ReferenceURL string    `dynamodbav:"referenceURL"`
					Spec         string    `dynamodbav:"logSpec"`
					Disabled     bool      `dynamodbav:"IsDeleted"`
				}{
					UpdatedAt:    record.UpdatedAt,
					Revision:     record.Revision,
					Release:      record.Release,
					Description:  record.Description,
					ReferenceURL: record.ReferenceURL,
					Spec:         record.Spec,
					Disabled:     record.Disabled,
				},
			},
			Condition: expression.Or(
				expression.Name(attrRecordKind).AttributeNotExists(),
				expression.And(
					// Check that the record is managed or user-defined based on input
					expression.Name(attrManaged).Equal(expression.Value(record.Managed)),
					// Check that the record has not incremented its revision
					expression.Name(attrRevision).Equal(expression.Value(currentRevision)),
				),
			),
			// Possible failures of the condition are
			// - The record is not managed
			// - The record is already at a newer release
			// To distinguish between the two we need to get the record values and check its revision and deleted attrs
			ReturnValuesOnConditionCheckFailure: dynamodb.ReturnValueAllOld,
			// We convert these failures to APIErrors here
			Cancel: func(r *dynamodb.CancellationReason) error {
				if transact.IsConditionalCheckFailed(r) {
					rec := ddbSchemaRecord{}
					if e := dynamodbattribute.UnmarshalMap(r.Item, &rec); e != nil {
						return e
					}
					if rec.Managed != record.Managed {
						return NewAPIError(ErrRevisionConflict, fmt.Sprintf("schema record %q is not managed", rec.RecordID))
					}
					if rec.Revision != currentRevision {
						return NewAPIError(ErrRevisionConflict, fmt.Sprintf("schema record %q is at revision %d", rec.RecordID, rec.Revision))
					}
				}
				return nil
			},
		},
	}
}

type recordKey struct {
	RecordID   string `json:"RecordID" validate:"required"`
	RecordKind string `json:"RecordKind" validate:"required,oneof=native status custom"`
}

func mustMarshalMap(val interface{}) map[string]*dynamodb.AttributeValue {
	attr, err := dynamodbattribute.MarshalMap(val)
	if err != nil {
		panic(err)
	}
	return attr
}
func schemaRecordKey(id string) recordKey {
	return recordKey{
		RecordID:   schemaRecordID(id),
		RecordKind: recordKindSchema,
	}
}

func schemaRecordID(id string) string {
	return strings.ToUpper(id)
}

type ddbSchemaRecord struct {
	recordKey
	SchemaRecord
}
