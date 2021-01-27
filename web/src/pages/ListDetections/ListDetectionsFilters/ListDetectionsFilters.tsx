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

import React from 'react';
import urls from 'Source/urls';
import { Form, Formik, FastField } from 'formik';
import {
  SortDirEnum,
  ListDetectionsInput,
  ListDetectionsSortFieldsEnum,
  DetectionTypeEnum,
} from 'Generated/schema';
import { Box, Flex } from 'pouncejs';
import pick from 'lodash/pick';
import useRequestParamsWithPagination from 'Hooks/useRequestParamsWithPagination';
import FormikAutosave from 'Components/utils/Autosave';
import FormikCombobox from 'Components/fields/ComboBox';
import FormikTextInput from 'Components/fields/TextInput';
import LinkButton from 'Components/buttons/LinkButton';
import DropdownFilters from './DropdownFilters';

export type ListDetectionsInlineFiltersValues = Pick<ListDetectionsInput, 'sortBy' | 'sortDir'>;

export type SortingOptions = {
  opt: string;
  resolution: ListDetectionsInput;
}[];

const filters = ['nameContains', 'sortBy', 'sortDir'] as (keyof ListDetectionsInput)[];

const defaultValues = {
  nameContains: '',
  sorting: null,
};

const sortingOpts: SortingOptions = [
  {
    opt: 'Name Ascending',
    resolution: {
      sortBy: ListDetectionsSortFieldsEnum.DisplayName,
      sortDir: SortDirEnum.Ascending,
    },
  },
  {
    opt: 'Name Descending',
    resolution: {
      sortBy: ListDetectionsSortFieldsEnum.DisplayName,
      sortDir: SortDirEnum.Descending,
    },
  },
  {
    opt: 'Most Recently Modified',
    resolution: {
      sortBy: ListDetectionsSortFieldsEnum.LastModified,
      sortDir: SortDirEnum.Descending,
    },
  },
  {
    opt: 'Oldest Modified',
    resolution: {
      sortBy: ListDetectionsSortFieldsEnum.LastModified,
      sortDir: SortDirEnum.Ascending,
    },
  },
  {
    opt: 'Severity Ascending',
    resolution: {
      sortBy: ListDetectionsSortFieldsEnum.Severity,
      sortDir: SortDirEnum.Ascending,
    },
  },
  {
    opt: 'Severity Descending',
    resolution: {
      sortBy: ListDetectionsSortFieldsEnum.Severity,
      sortDir: SortDirEnum.Descending,
    },
  },
];

const sortingItems = sortingOpts.map(sortingOption => sortingOption.opt);
const detectionTypeItems = Object.values(DetectionTypeEnum);

/**
 * Since sorting is not responding to some ListDetectionsInput key we shall extract
 * this information from `sortBy` and `sortDir` parameters in order to align the
 * combobox values.
 */
const extractSortingOpts = params => {
  const { sorting, ...rest } = params;
  const sortingParams = sortingOpts.find(param => param.opt === sorting);
  return {
    ...rest,
    ...(sortingParams ? { ...sortingParams.resolution } : {}),
  };
};

const wrapSortingOptions = params => {
  const { sortBy, sortDir, ...rest } = params;
  const option = sortingOpts.find(
    param => param.resolution.sortBy === sortBy && param.resolution.sortDir === sortDir
  );

  return {
    ...(option ? { sorting: option.opt } : {}),
    ...rest,
  };
};

const ListRuleFilters: React.FC = () => {
  const { requestParams, updateRequestParamsAndResetPaging } = useRequestParamsWithPagination<
    ListDetectionsInput
  >();
  const initialFilterValues = React.useMemo(
    () =>
      ({
        ...defaultValues,
        ...wrapSortingOptions(pick(requestParams, filters)),
      } as ListDetectionsInlineFiltersValues),
    [requestParams]
  );
  return (
    <Flex justify="flex-end" align="center" spacing={4}>
      <Formik<ListDetectionsInlineFiltersValues>
        enableReinitialize
        initialValues={initialFilterValues}
        onSubmit={(values: ListDetectionsInlineFiltersValues) => {
          updateRequestParamsAndResetPaging(extractSortingOpts(values));
        }}
      >
        <Form>
          <FormikAutosave threshold={200} />
          <Flex spacing={4} align="center" pr={4}>
            <Box minWidth={410}>
              <FastField
                name="nameContains"
                icon="search"
                iconAlignment="left"
                as={FormikTextInput}
                label="Filter Rules by text"
                placeholder="Search for a rule..."
              />
            </Box>
            <Box minWidth={150}>
              <FastField
                name="sorting"
                as={FormikCombobox}
                items={sortingItems}
                label="Sort By"
                placeholder="Select a sort option"
              />
            </Box>
            <Box minWidth={150}>
              <FastField
                name="analysisType"
                as={FormikCombobox}
                items={detectionTypeItems}
                label="Detection Type"
                placeholder="Select a type"
              />
            </Box>
          </Flex>
        </Form>
      </Formik>
      <DropdownFilters />
      <LinkButton to={urls.detections.create()}>Create New Detection</LinkButton>
    </Flex>
  );
};

export default React.memo(ListRuleFilters);
