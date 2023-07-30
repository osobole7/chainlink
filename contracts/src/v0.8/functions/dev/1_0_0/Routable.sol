// SPDX-License-Identifier: MIT

pragma solidity ^0.8.19;

import {IConfigurable} from "./interfaces/IConfigurable.sol";
import {ITypeAndVersion} from "../../../shared/interfaces/ITypeAndVersion.sol";
import {IRouterBase} from "./interfaces/IRouterBase.sol";
import {IOwnable} from "../../../shared/interfaces/IOwnable.sol";

abstract contract Routable is ITypeAndVersion, IConfigurable {
  bytes32 internal s_configHash;

  IRouterBase internal s_router;

  error RouterMustBeSet();
  error OnlyCallableByRouter();
  error OnlyCallableByRouterOwner();

  /**
   * @dev Initializes the contract.
   */
  constructor(address router, bytes memory config) {
    if (router == address(0)) {
      revert RouterMustBeSet();
    }
    s_router = IRouterBase(router);
    updateConfig(config);
  }

  /**
   * @inheritdoc IConfigurable
   */
  function getConfigHash() external view override returns (bytes32 config) {
    return s_configHash;
  }

  /**
   * @dev Must be implemented by inheriting contract
   * Use to set configuration state
   */
  function _updateConfig(bytes memory config) internal virtual;

  /**
   * @inheritdoc IConfigurable
   */
  function updateConfig(bytes memory config) public override onlyRouter {
    _updateConfig(config);
    s_configHash = keccak256(config);
  }

  /**
   * @notice Reverts if called by anyone other than the router.
   */
  modifier onlyRouter() {
    if (msg.sender != address(s_router)) {
      revert OnlyCallableByRouter();
    }
    _;
  }

  /**
   * @notice Reverts if called by anyone other than the router owner.
   */
  modifier onlyRouterOwner() {
    if (msg.sender != IOwnable(address(s_router)).owner()) {
      revert OnlyCallableByRouterOwner();
    }
    _;
  }
}
