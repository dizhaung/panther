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
import { ButtonProps, Button } from 'pouncejs';
import LinkButtonWrapper, { LinkButtonWrapperProps } from '../utils/LinkButtonWrapper';

type LinkButtonProps = LinkButtonWrapperProps & Omit<ButtonProps, 'as'>;

const LinkButton: React.FC<LinkButtonProps> = ({ disabled, external, to, children, ...rest }) => {
  return (
    <LinkButtonWrapper disabled={disabled} external={external} to={to}>
      <Button as="span" aria-disabled={disabled} {...rest}>
        {children}
      </Button>
    </LinkButtonWrapper>
  );
};
export default LinkButton;
