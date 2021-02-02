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

	"github.com/pkg/errors"
	"go.uber.org/multierr"
	"gopkg.in/yaml.v2"

	"github.com/panther-labs/panther/internal/log_analysis/log_processor/customlogs"
	"github.com/panther-labs/panther/internal/log_analysis/log_processor/logschema"
	"github.com/panther-labs/panther/pkg/stringset"
)

// GetCustomLog gets a custom log record for the specified id and revision
func (api *LogTypesAPI) GetCustomLog(ctx context.Context, input *GetCustomLogInput) (*GetCustomLogOutput, error) {
	id := customlogs.LogType(input.LogType)
	r, err := api.Database.GetSchema(ctx, id, input.Revision)
	if err != nil {
		return nil, err
	}
	if r == nil || !r.IsCustom() || r.Disabled {
		return nil, NewAPIError(ErrNotFound, fmt.Sprintf("custom log record %q not found at revision %d", input.LogType, input.Revision))
	}
	// Return compatible type
	return &GetCustomLogOutput{
		Result: r,
	}, nil
}

// GetCustomLogInput specifies the log type id and revision to retrieve.
// Zero Revision will get the latest revision of the log type record
type GetCustomLogInput struct {
	LogType  string `json:"logType" validate:"required,startswith=Custom." description:"The log type id"`
	Revision int64  `json:"revision,omitempty" validate:"omitempty,min=1" description:"Log record revision (0 means latest)"`
}
type GetCustomLogOutput struct {
	Result *SchemaRecord `json:"record,omitempty" description:"The custom log record (field omitted if an error occurred)"`
	Error  *APIError     `json:"error,omitempty" description:"An error that occurred while fetching the record"`
}

func (api *LogTypesAPI) PutCustomLog(ctx context.Context, input *PutCustomLogInput) (*PutCustomLogOutput, error) {
	id := customlogs.LogType(input.LogType)
	schema, err := buildSchema(id, input.Description, input.ReferenceURL, input.Spec)
	if err != nil {
		return nil, err
	}
	schema.Schema = id

	switch currentRevision := input.Revision; currentRevision {
	case 0:
		result, err := api.Database.CreateUserSchema(ctx, id, input.SchemaUpdate)
		if err != nil {
			return nil, err
		}
		if err := api.UpdateDataCatalog(ctx, input.LogType, nil, schema.Fields); err != nil {
			// The error will be shown to the user as a "ServerError"
			return nil, errors.Wrapf(err, "could not queue event for %q database update", input.LogType)
		}
		return &PutCustomLogOutput{
			Result: result,
		}, nil
	default:
		current, err := api.Database.GetSchema(ctx, id, 0)
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, NewAPIError(ErrNotFound, fmt.Sprintf("record %q was not found", id))
		}
		if !current.IsCustom() {
			return nil, NewAPIError(ErrNotFound, fmt.Sprintf("record %q was not found", id))
		}
		if current.Revision != currentRevision {
			return nil, NewAPIError(ErrRevisionConflict, fmt.Sprintf("record %q is not on revision %d", id, currentRevision))
		}

		currentSchema, err := buildSchema(current.Name, current.Description, current.ReferenceURL, current.Spec)
		if err != nil {
			return nil, err
		}

		if err := api.checkUpdate(currentSchema, schema); err != nil {
			return nil, NewAPIError(ErrInvalidUpdate, fmt.Sprintf("schema update is not backwards compatible: %s", err))
		}
		result, err := api.Database.UpdateUserSchema(ctx, id, current.Revision, input.SchemaUpdate)
		if err != nil {
			return nil, err
		}
		if err := api.UpdateDataCatalog(ctx, input.LogType, currentSchema.Fields, schema.Fields); err != nil {
			// The error will be shown to the user as a "ServerError"
			return nil, errors.Wrapf(err, "could not queue event for %q database update", input.LogType)
		}
		return &PutCustomLogOutput{
			Result: result,
		}, nil
	}
}

func (api *LogTypesAPI) checkUpdate(a, b *logschema.Schema) error {
	diff, err := logschema.Diff(a, b)
	if err != nil {
		return err
	}
	for i := range diff {
		c := &diff[i]
		if e := customlogs.CheckSchemaChange(c); e != nil {
			err = multierr.Append(err, e)
		}
	}
	return err
}

func buildSchema(name, desc, refURL, spec string) (*logschema.Schema, error) {
	schema := logschema.Schema{}
	if err := yaml.Unmarshal([]byte(spec), &schema); err != nil {
		return nil, NewAPIError(ErrInvalidSyntax, err.Error())
	}
	if desc != "" {
		schema.Description = desc
	}
	if refURL != "" {
		schema.ReferenceURL = refURL
	}
	if err := logschema.ValidateSchema(&schema); err != nil {
		return nil, NewAPIError(ErrInvalidLogSchema, err.Error())
	}
	// Schemas requiring native parsers don't need further checks
	if p := schema.Parser; p != nil && p.Native != nil {
		return &schema, nil
	}
	// Build non-native parser entries
	if _, err := customlogs.Build(name, &schema); err != nil {
		return nil, err
	}
	return &schema, nil
}

// nolint:lll
type PutCustomLogInput struct {
	LogType string `json:"logType" validate:"required,startswith=Custom." description:"The log type id"`
	// Revision is required when updating a custom log record.
	// If  is omitted a new custom log record will be created.
	Revision int64 `json:"revision,omitempty" validate:"omitempty,min=1" description:"Custom log record revision to update (if omitted a new record will be created)"`
	SchemaUpdate
}

//nolint:lll
type PutCustomLogOutput struct {
	Result *SchemaRecord `json:"record,omitempty" description:"The modified record (field is omitted if an error occurred)"`
	Error  *APIError     `json:"error,omitempty" description:"An error that occurred during the operation"`
}

func (api *LogTypesAPI) DelCustomLog(ctx context.Context, input *DelCustomLogInput) (*DelCustomLogOutput, error) {
	inUse, err := api.LogTypesInUse(ctx)
	if err != nil {
		return nil, err
	}

	if stringset.Contains(inUse, input.LogType) {
		return nil, NewAPIError(ErrInUse, fmt.Sprintf("log %s in use", input.LogType))
	}

	id := customlogs.LogType(input.LogType)
	if err := api.Database.ToggleSchema(ctx, id, false); err != nil {
		return nil, err
	}
	if err := api.UpdateDataCatalog(ctx, input.LogType, nil, nil); err != nil {
		// The error will be shown to the user as a "ServerError"
		return nil, errors.Wrapf(err, "could not queue event for %q database update", input.LogType)
	}
	return &DelCustomLogOutput{}, nil
}

type DelCustomLogInput struct {
	LogType  string `json:"logType" validate:"required,startswith=Custom." description:"The log type id"`
	Revision int64  `json:"revision" validate:"min=1" description:"Log record revision"`
}

type DelCustomLogOutput struct {
	Error *APIError `json:"error,omitempty" description:"The delete record"`
}

func (api *LogTypesAPI) ListCustomLogs(ctx context.Context) (*ListCustomLogsOutput, error) {
	records := []*SchemaRecord{}
	scan := func(r *SchemaRecord) bool {
		if r.IsCustom() {
			records = append(records, r)
		}
		return true
	}
	if err := api.Database.ScanSchemas(ctx, scan); err != nil {
		return nil, err
	}
	return &ListCustomLogsOutput{
		Records: records,
	}, nil
}

//nolint:lll
type ListCustomLogsOutput struct {
	Records []*SchemaRecord `json:"customLogs" description:"Custom log records stored"`
	Error   *APIError       `json:"error,omitempty" description:"An error that occurred during the operation"`
}
